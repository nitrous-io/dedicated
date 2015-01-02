package user

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
)

const resetPasswordWithin = 4 * time.Hour
const maxConfirmationHours = 72 * time.Hour
const maxUnconfirmedDuration = 24 * time.Hour

var (
	usernameRe               = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*\z`)
	emailRe                  = regexp.MustCompile(`\A[^@\s]+@([^@\s]+\.)+[^@\s]+\z`)
	Mailer     mailer.Mailer = mailer.NewSendGridMailer()
)

var (
	ErrEmailBlank       = errors.New("email can't be blank")
	ErrEmailInvalid     = errors.New("email is not valid")
	ErrUsernameBlank    = errors.New("username can't be blank")
	ErrUsernameTooLong  = errors.New("username is too long (maximum is 30 characters)")
	ErrUsernameInvalid  = errors.New("username may only contain alphanumeric characters or dashes and cannot begin with a dash")
	ErrPasswordBlank    = errors.New("password can't be blank")
	ErrPasswordTooShort = errors.New("password is too short (minimum is 6 characters)")
	ErrPasswordTooLong  = errors.New("password is too long (maximum is 30 characters)")

	ErrConfirmationLapsed           = errors.New("confirmation has expired")
	ErrConfirmationAlreadyCompleted = errors.New("email has already been confirmed")
)

type User struct {
	ID        int64      `db:"id" json:"id"`
	Name      string     `db:"name" json:"name"`
	Username  string     `db:"username" json:"username"`
	Email     string     `db:"email" json:"email"`
	CreatedAt *time.Time `db:"created_at" json:"created_at"`
	UpdatedAt *time.Time `db:"updated_at" json:"updated_at"`

	Password          string `json:"-"`
	EncryptedPassword string `db:"encrypted_password" json:"-"`

	LoginAttempts  int           `db:"login_attempts" json:"-"`
	InviteCodeID   sql.NullInt64 `db:"invite_code_id" json:"-"`
	Admin          bool          `db:"admin" json:"admin"`
	BillingEnabled bool          `db:"billing_enabled" json:"billing_enabled"`

	ResetPasswordToken  sql.NullString `db:"reset_password_token" json:"-"`
	ResetPasswordSentAt *time.Time     `db:"reset_password_sent_at" json:"-"`

	ConfirmationToken  sql.NullString `db:"confirmation_token" json:"-"`
	ConfirmationSentAt *time.Time     `db:"confirmation_sent_at" json:"-"`
	ConfirmedAt        *time.Time     `db:"confirmed_at" json:"-"`
	UnconfirmedEmail   sql.NullString `db:"unconfirmed_email" json:"unconfirmed_email,omitempty"`
}

func FindByID(d db.QueryReceiver, id int64) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE id = $1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func FindByUsername(d db.QueryReceiver, username string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE username = $1", username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func FindByUsernameOrEmail(d db.QueryReceiver, login string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE username = $1 OR email = $1", login)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func FindByResetPasswordToken(d db.QueryReceiver, token string) (*User, error) {
	user := &User{}
	lastValidSentAt := time.Now().Add(-resetPasswordWithin)
	err := d.Get(user, "SELECT * FROM users WHERE reset_password_token = $1 AND reset_password_sent_at > $2", token, lastValidSentAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func FindByConfirmationToken(d db.QueryReceiver, token string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE confirmation_token = $1", token)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// GetDelay returns a time.Duration based on no. of login attempts made on
// given login.
func GetDelay(d db.QueryReceiver, login string) (time.Duration, error) {
	var attemptCount int

	var (
		attempts = []int{3, 6, 9}
		delay    = []time.Duration{
			time.Duration(0),
			time.Second * 1,
			time.Second * 3,
		}
		finalDelay = time.Second * 5
		noDelay    = time.Duration(0)
	)

	err := d.QueryRowx("SELECT login_attempts FROM users WHERE username = $1 OR email = $1", login).Scan(&attemptCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return noDelay, nil
		}
		return noDelay, err
	}

	if attemptCount == 0 {
		return noDelay, nil
	}

	for i, a := range attempts {
		if attemptCount < a {
			return delay[i], nil
		}
	}
	return finalDelay, nil
}

func IncrementLoginAttempts(d db.QueryReceiver, login string) error {
	_, err := d.Exec("UPDATE users SET login_attempts = least(login_attempts + 1, 32767) WHERE username = $1 OR email = $1", login)
	return err
}

func ResetLoginAttempts(d db.QueryReceiver, login string) error {
	_, err := d.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1 OR email = $1", login)
	return err
}

func (u *User) Insert(tx *sqlx.Tx) error {
	if err := u.encryptPassword(); err != nil {
		return err
	}
	nstmt, err := tx.PrepareNamed(`INSERT INTO users
		(email, username, encrypted_password, name, invite_code_id)
		VALUES (:email, :username, :encrypted_password, :name, :invite_code_id)
		RETURNING *;`)
	if err != nil {
		return err
	}

	if err = nstmt.QueryRow(u).StructScan(u); err != nil {
		return err
	}

	// create a user profile
	if err := userprofile.Create(tx, u.ID); err != nil {
		return err
	}

	// create a billing profile
	return billingprofile.Create(tx, u.ID)
}

func (u *User) Update(d db.QueryReceiver) error {
	if err := u.encryptPassword(); err != nil {
		return err
	}
	nstmt, err := d.PrepareNamed(`UPDATE users
		SET (name, email, encrypted_password, unconfirmed_email, updated_at)
		= (:name, :email, :encrypted_password, :unconfirmed_email, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(u).StructScan(u)
}

func (u *User) Validate() error {
	if u.Email == "" {
		return ErrEmailBlank
	}
	if !emailRe.MatchString(u.Email) {
		return ErrEmailInvalid
	}

	if u.Username == "" {
		return ErrUsernameBlank
	}
	usernameLen := len(u.Username)
	if usernameLen > 30 {
		return ErrUsernameTooLong
	}
	if !usernameRe.MatchString(u.Username) {
		return ErrUsernameInvalid
	}

	if u.Password == "" && u.EncryptedPassword == "" {
		return ErrPasswordBlank
	}
	if u.Password != "" {
		passwordLen := len(u.Password)
		if passwordLen < 6 {
			return ErrPasswordTooShort
		}
		if passwordLen > 30 {
			return ErrPasswordTooLong
		}
	}

	return nil
}

func (u *User) Topic() pubsub.Topic {
	return pubsub.Topic(fmt.Sprintf("user-%d", u.ID))
}

func (u *User) isPaid(d db.QueryReceiver) (bool, error) {
	var b bool
	if err := d.QueryRowx(`SELECT state = $1 FROM billing_profiles WHERE user_id = $2;`, billingprofile.StatePaid, u.ID).Scan(&b); err != nil {
		return false, err
	}
	return b, nil
}

func (u *User) CanCreateHosts(d db.QueryReceiver) (bool, error) {
	// check if the user is a paid user
	paid, err := u.isPaid(d)
	if err != nil {
		return false, err
	}
	if paid {
		return true, nil
	}

	// user is not a paid user, check the max hosts she can create
	var b bool
	err = d.QueryRowx(`SELECT (count(h.id) < coalesce(min(up.max_hosts), 0))
		FROM users u
		JOIN user_profiles up ON (up.user_id = u.id)
		LEFT JOIN hosts h ON (h.user_id = u.id AND h.terminated_at IS NULL)
		WHERE u.id = $1;`,
		u.ID).Scan(&b)

	if err != nil {
		return false, err
	}
	return b, nil
}

func (u *User) HasFailedPayments(d db.QueryReceiver) (bool, error) {
	var b bool
	if err := d.QueryRowx(`SELECT failed FROM invoices WHERE user_id = $1 AND failed = TRUE LIMIT 1;`, u.ID).Scan(&b); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return b, nil
}

func (u *User) ConfirmationLapsed() bool {
	if u.ConfirmationSentAt == nil {
		return false
	}
	return u.ConfirmationSentAt.Add(maxUnconfirmedDuration).Before(time.Now())
}

func (u *User) SetResetPasswordToken(d db.QueryReceiver) error {
	t, err := securerandom.Base64(191)
	if err != nil {
		return err
	}
	u.ResetPasswordToken = sql.NullString{t[:255], true}

	nstmt, err := d.PrepareNamed("UPDATE users SET (reset_password_token, reset_password_sent_at) = (:reset_password_token, now()) WHERE id = :id RETURNING *;")
	if err != nil {
		return err
	}
	err = nstmt.QueryRowx(u).StructScan(u)
	if err != nil {
		return err
	}
	return nil
}

func (u *User) encryptPassword() error {
	if u.Password == "" {
		return nil
	}
	encrypted, err := bcrypt.Hash(u.Password)
	if err != nil {
		return err
	}
	u.EncryptedPassword = encrypted
	u.Password = ""
	return nil
}

func (u *User) SendConfirmationEmail(d db.QueryReceiver, setConfirmationSentAt, flushConfirmedAt bool) error {
	confirmedAtOp := "confirmed_at"
	if flushConfirmedAt {
		u.ConfirmedAt = nil
		confirmedAtOp = "NULL"
	}

	if u.ConfirmedAt != nil {
		return ErrConfirmationAlreadyCompleted
	}

	t, err := securerandom.Base64(191)
	if err != nil {
		return err
	}
	u.ConfirmationToken = sql.NullString{t[:255], true}

	var stmt string
	if setConfirmationSentAt || u.ConfirmationSentAt == nil {
		stmt = "UPDATE users SET (confirmation_token, confirmation_sent_at, confirmed_at) = (:confirmation_token, now(), %s) WHERE id = :id RETURNING *;"
	} else {
		stmt = "UPDATE users SET (confirmation_token, confirmed_at) = (:confirmation_token, %s) WHERE id = :id RETURNING *;"
	}

	// FIXME: ensure confirmation_token is unique and retry, do not surface error
	nstmt, err := d.PrepareNamed(fmt.Sprintf(stmt, confirmedAtOp))
	if err != nil {
		return err
	}
	err = nstmt.QueryRowx(u).StructScan(u)
	if err != nil {
		return err
	}

	host := envvars.WebHostname
	if host == "" {
		host = "pro.nitrous.io"
	}
	resetURL := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app",
		Fragment: fmt.Sprintf("confirm-email?token=%s", u.ConfirmationToken.String),
	}

	tmplData := struct {
		Name       string
		ConfirmUrl string
	}{
		u.Name,
		resetURL.String(),
	}

	// send email
	htmlBuf := bytes.NewBuffer(nil)
	if err := confirmEmailHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err := confirmEmailTxtTemplate.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	var targetEmail string
	if u.UnconfirmedEmail.String != "" {
		targetEmail = u.UnconfirmedEmail.String
	} else {
		targetEmail = u.Email
	}

	from := (&mail.Address{Name: "Nitrous Mailer", Address: "mailer@nitrous.io"}).String()
	to := (&mail.Address{Name: u.Name, Address: targetEmail}).String()

	return Mailer.SendMail(from, []string{to}, []string{from}, nil, from, "Nitrous - confirm your email", txtBuf.String(), htmlBuf.String())
}

func (u *User) ConfirmEmail(d db.QueryReceiver) error {
	if u.ConfirmedAt != nil {
		return ErrConfirmationAlreadyCompleted
	}

	maxExpiry := u.ConfirmationSentAt.Add(maxConfirmationHours)
	if time.Now().After(maxExpiry) {
		return ErrConfirmationLapsed
	}

	if u.UnconfirmedEmail.String != "" {
		u.Email = u.UnconfirmedEmail.String
		u.UnconfirmedEmail.String = ""
	}

	nstmt, err := d.PrepareNamed(`UPDATE users
		SET (email, unconfirmed_email,confirmation_token, confirmed_at, confirmation_sent_at)
		= (:email, NULL, NULL, now(), NULL)
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(u).StructScan(u)
}

func (u *User) SendPasswordResetEmail(d db.QueryReceiver) error {
	err := u.SetResetPasswordToken(d)
	if err != nil {
		return err
	}

	from := (&mail.Address{Name: "Nitrous", Address: "support@nitrous.io"}).String()
	to := (&mail.Address{Name: u.Name, Address: u.Email}).String()
	replyTo := from
	subject := fmt.Sprintf("[nitrous-pro] Reset password instructions")

	host := envvars.WebHostname
	if host == "" {
		host = "pro.nitrous.io"
	}
	resetURL := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app",
		Fragment: fmt.Sprintf("/reset-password?token=%s", u.ResetPasswordToken.String),
	}
	name := u.Username
	if u.Name != "" {
		name = u.Name
	}
	params := map[string]string{"Name": name, "URL": resetURL.String()}

	var htmlBuf, txtBuf bytes.Buffer
	err = resetPasswordEmailHtmlTemplate.Execute(&htmlBuf, params)
	if err != nil {
		return err
	}
	err = resetPasswordEmailTxtTemplate.Execute(&txtBuf, params)
	if err != nil {
		return err
	}

	err = Mailer.SendMail(from, []string{to}, nil, nil, replyTo, subject, txtBuf.String(), htmlBuf.String())
	if err != nil {
		return err
	}

	return nil
}

// ActivateBillingProfile will create a new customer in Stripe and return the updated billing profile.
func (u *User) ActivateBillingProfile(d db.QueryReceiver, token string) (*billingprofile.BillingProfile, error) {
	if !u.BillingEnabled {
		return nil, fmt.Errorf("Billing not enabled for the given user - %s", u.Username)
	}

	params := &stripe.CustomerParams{
		Token: token,
		Email: u.Email,
		Desc:  fmt.Sprint(u.Name, u.Username),
	}
	c, err := customer.New(params)
	if err != nil {
		return nil, billingprofile.HandleStripeError(err)
	}

	// update the profile with details returned from Stripe
	crd := c.Cards.Values[0]
	b := &billingprofile.BillingProfile{
		UserID:   u.ID,
		StripeID: sql.NullString{c.ID, true},
		State:    "paid",
		LastFour: sql.NullString{crd.LastFour, true},
		CardType: sql.NullString{string(crd.Brand), true},
		ExpMonth: sql.NullInt64{int64(crd.Month), true},
		ExpYear:  sql.NullInt64{int64(crd.Year), true},
	}

	err = b.Update(d)
	if err != nil {
		return nil, fmt.Errorf("Billing profile update failed for user %s. Reason - %s", u.Username, err)
	}
	return b, nil
}

package user

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/blacklist"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailchimp"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
)

const resetPasswordWithin = 4 * time.Hour
const maxConfirmationHours = 72 * time.Hour
const maxUnconfirmedDuration = 24 * time.Hour

const (
	USAGE_NOTICES_NO_WARNING = iota
	USAGE_NOTICES_FIRST_WARNING
	USAGE_NOTICES_SECOND_WARNING
)

var (
	usernameRe = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*\z`)
	emailRe    = regexp.MustCompile(`\A[^@\s]+@([^@\s]+\.)+[^@\s]+\z`)
	Mailer     mailer.Mailer
	MailChimp  mailchimp.MailChimpProvider

	AllowedDesktopAppBillingStates = []string{billingprofile.StatePaid, billingprofile.StateExempt, billingprofile.StatePaymentDue}
)

var (
	ErrEmailBlank       = errors.New("email can't be blank")
	ErrEmailInvalid     = errors.New("email is not valid")
	ErrEmailBlacklisted = errors.New("email is blacklisted")
	ErrUsernameBlank    = errors.New("username can't be blank")
	ErrUsernameTooLong  = errors.New("username is too long (maximum is 30 characters)")
	ErrUsernameInvalid  = errors.New("username may only contain alphanumeric characters or dashes and cannot begin with a dash")
	ErrPasswordBlank    = errors.New("password can't be blank")
	ErrPasswordTooShort = errors.New("password is too short (minimum is 6 characters)")
	ErrPasswordTooLong  = errors.New("password is too long (maximum is 100 characters)")
	ErrPhoneInvalid     = errors.New("phone number is not valid")
	ErrCardBanned       = errors.New("credit card is banned")

	ErrMaxHostsReached            = errors.New("reached maximum number of hosts")
	ErrNoCard                     = errors.New("not added credit card")
	ErrResetPasswordTokenRequired = errors.New("reset password token is required")

	ErrConfirmationLapsed           = errors.New("confirmation has expired")
	ErrConfirmationAlreadyCompleted = errors.New("email has already been confirmed")
)

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
	MailChimp = &mailchimp.MailChimp{}
}

type User struct {
	ID        int64      `db:"id"`
	Name      string     `db:"name"`
	Username  string     `db:"username"`
	Email     string     `db:"email"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`

	FirstTime  bool           `db:"first_time"`
	Suspicious sql.NullString `db:"suspicious"`

	Password          string
	EncryptedPassword string `db:"encrypted_password"`

	LoginAttempts  int  `db:"login_attempts"`
	Admin          bool `db:"admin"`
	ServiceAccount bool `db:"service_account"`

	MaxContainers sql.NullInt64 `db:"max_containers"`

	// only applicable when true, which overrides the logic
	// for determining use of desktop app
	CanUseDesktopApp sql.NullBool `db:"can_use_desktop_app"`

	ResetPasswordToken  sql.NullString `db:"reset_password_token"`
	ResetPasswordSentAt *time.Time     `db:"reset_password_sent_at"`

	ConfirmationToken  sql.NullString `db:"confirmation_token"`
	ConfirmationSentAt pq.NullTime    `db:"confirmation_sent_at"`
	ConfirmedAt        pq.NullTime    `db:"confirmed_at"`
	UnconfirmedEmail   sql.NullString `db:"unconfirmed_email"`

	Phone            sql.NullString `db:"phone"`
	PhoneCountryCode sql.NullString `db:"phone_country_code"`
	PhoneAuthed      bool           `db:"phone_authed"`

	DeletedAt pq.NullTime `db:"deleted_at"`

	BannedAt   pq.NullTime   `db:"banned_at"`
	BannedByID sql.NullInt64 `db:"banned_by_id"`

	// UsageQuota is the number of seconds that a user is allowed to use a free
	// host.
	UsageQuota sql.NullInt64 `db:"usage_quota"`
	// UsageQuotaPeriod is the number of seconds after which the free host usage
	// quota is reset to full.
	UsageQuotaPeriod     sql.NullInt64 `db:"usage_quota_period"`
	UsageRemaining       sql.NullInt64 `db:"usage_remaining"`
	UsageQuotaRestoredAt pq.NullTime   `db:"usage_quota_restored_at"`
	UsageNoticesSent     sql.NullInt64 `db:"usage_notices_sent"`

	CreatedByClientID sql.NullInt64 `db:"created_by_client_id"`

	// cached billing profile
	billingProfile *billingprofile.BillingProfile

	// deprecated
	PreviousPromoCode sql.NullString `db:"previous_promo_code"`
}

func (u *User) BillingProfile(d db.QueryReceiver) (*billingprofile.BillingProfile, error) {
	if u.billingProfile == nil {
		bp, err := billingprofile.Find(d, u.ID)
		if err != nil {
			return nil, err
		}

		u.billingProfile = bp
	}

	return u.billingProfile, nil
}

// RestoreUsageQuotas restores the usage_remaining (to the usage_quota value)
// for all users who:
// 1. have a free host, and
// 2. had their free usage quota last restored earlier than their usage quota
//    period.
func RestoreUsageQuotas(d db.QueryReceiver, when time.Time) (int64, error) {
	res, err := d.Exec(`UPDATE users
		SET
			usage_remaining = usage_quota,
			usage_quota_restored_at = now()
		WHERE id IN (
			SELECT users.id FROM
			users JOIN hosts
				ON users.id = hosts.user_id
			WHERE
				hosts.free = true
				AND hosts.terminated_at IS NULL
				AND (usage_quota_restored_at
					+ (INTERVAL '1 second' * usage_quota_period))
						<= $1 -- (last restore date + quota period) is before "now"
		)`, when)
	if err != nil {
		return 0, err
	}
	rowsAffected, _ := res.RowsAffected()
	return rowsAffected, nil
}

func FindByID(d db.QueryReceiver, id int64) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL", id)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func FindByUsername(d db.QueryReceiver, username string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE username = $1 AND deleted_at IS NULL", username)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func FindByEmail(d db.QueryReceiver, email string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL", email)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func FindByUsernameOrEmail(d db.QueryReceiver, login string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE (username = $1 OR email = $1) AND deleted_at IS NULL", login)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func FindByResetPasswordToken(d db.QueryReceiver, token string) (*User, error) {
	if token == "" {
		return nil, ErrResetPasswordTokenRequired
	}
	user := &User{}
	lastValidSentAt := time.Now().Add(-resetPasswordWithin)
	err := d.Get(user, "SELECT * FROM users WHERE reset_password_token = $1 AND reset_password_sent_at > $2 AND deleted_at IS NULL", token, lastValidSentAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func FindByConfirmationToken(d db.QueryReceiver, token string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE confirmation_token = $1 AND deleted_at IS NULL", token)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func FindAllByUsageAndNoticeSent(d db.QueryReceiver, usageRemaining, noticeSent int64) ([]*User, error) {
	users := []*User{}
	err := d.Select(&users, `
		SELECT * FROM users
		WHERE usage_remaining <= $1
		AND usage_notices_sent = $2
		ORDER BY id
	`, usageRemaining, noticeSent)
	if err != nil {
		return nil, err
	}
	return users, nil
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

	u.CheckSuspicious()

	nstmt, err := tx.PrepareNamed(`INSERT INTO users
		(email, username, encrypted_password, name, suspicious, created_by_client_id)
		VALUES (:email, :username, :encrypted_password, :name, :suspicious, :created_by_client_id)
		RETURNING *;`)
	if err != nil {
		return err
	}
	if err := nstmt.QueryRow(u).StructScan(u); err != nil {
		return err
	}

	if err := u.CreateSettings(tx); err != nil {
		return err
	}

	if err := userprofile.Create(tx, u.ID); err != nil {
		return err
	}

	return billingprofile.Create(tx, u.ID, sql.NullInt64{Valid: false})
}

func (u *User) Update(d db.QueryReceiver) error {
	if err := u.encryptPassword(); err != nil {
		return err
	}
	nstmt, err := d.PrepareNamed(`UPDATE users
		SET (name, email, encrypted_password,
			unconfirmed_email, confirmation_sent_at, confirmation_token,
			usage_quota_restored_at,
			updated_at)
		= (:name, :email, :encrypted_password,
			:unconfirmed_email, :confirmation_sent_at, :confirmation_token,
			:usage_quota_restored_at,
			now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(u).StructScan(u)
}

func (u *User) UpdatePhone(d db.QueryReceiver, phone, country_code string) error {
	// Both phone and country code are required.
	if phone == "" || country_code == "" {
		return ErrPhoneInvalid
	}

	// `phone_authed = false` condition prevents phone number from being updated
	// if already been authed. This is because we use phone numbers to uniquely
	// identify users, so we just prevent it from being changed once it's been
	// authenticated. We might want to allow this to be changed in future.
	return d.QueryRowx(`UPDATE users
		SET phone = $1, phone_country_code = $2, updated_at = now()
		WHERE id = $3 AND phone_authed = $4
		RETURNING *`, phone, country_code, u.ID, false).StructScan(u)
}

func (u *User) UpdatePhoneAuthed(d db.QueryReceiver, b bool) error {
	// Both phone and country code are required.
	if !u.Phone.Valid || !u.PhoneCountryCode.Valid {
		return ErrPhoneInvalid
	}

	return d.QueryRowx(`UPDATE users
		SET phone_authed = $1, updated_at = now()
		WHERE id = $2
		RETURNING *`, b, u.ID).StructScan(u)
}

func (u *User) DecrementUsageRemaining(d db.QueryReceiver, secondsUsed int64) error {
	return d.QueryRowx(`UPDATE users
		SET usage_remaining = usage_remaining - $1, updated_at = now()
		WHERE id = $2
		RETURNING *`, secondsUsed, u.ID).StructScan(u)
}

func (u *User) Validate() error {
	if u.Email == "" {
		return ErrEmailBlank
	}
	if !emailRe.MatchString(u.Email) {
		return ErrEmailInvalid
	}
	if blacklist.Check(u.Email) {
		return ErrEmailBlacklisted
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
		if passwordLen > 100 {
			return ErrPasswordTooLong
		}
	}

	return nil
}

func (u *User) DisplayName() string {
	name := u.Name
	if name == "" {
		name = u.Username
	}
	return name
}

// PhoneNumber returns the phone number including a "+" and the country code in
// front, e.g. +16175551212 (US number) and +6588884444 (SG number).
func (u *User) PhoneNumber() string {
	if u.Phone.String == "" || u.PhoneCountryCode.String == "" {
		return ""
	}
	return fmt.Sprintf("+%s%s", u.PhoneCountryCode.String, u.Phone.String)
}

func (u *User) Topic() pubsub.Topic {
	return pubsub.Topic(fmt.Sprintf("user-%d", u.ID))
}

func (u *User) CanCreateHosts(d db.QueryReceiver) error {
	// Admins and service accounts have free rein
	if u.Admin || u.ServiceAccount {
		return nil
	}

	bp, err := u.BillingProfile(d)
	if err != nil {
		return err
	}
	if err = bp.CanCreateHosts(); err != nil {
		if err == billingprofile.ErrNoCard {
			return ErrNoCard
		}
		return err
	}

	// Ensure max_hosts limit has not been reached
	var b bool
	if err = d.QueryRowx(`SELECT (count(h.id) < coalesce(min(up.max_hosts), 0))
		FROM users u
		JOIN user_profiles up ON (up.user_id = u.id)
		LEFT JOIN hosts h ON (h.user_id = u.id AND h.terminated_at IS NULL)
		WHERE u.id = $1;`,
		u.ID).Scan(&b); err != nil {
		return err
	}
	if !b {
		return ErrMaxHostsReached
	}

	return nil
}

func (u *User) HasFailedPayments(d db.QueryReceiver) (bool, error) {
	var count int
	if err := d.QueryRowx(`SELECT COUNT(*) FROM invoices WHERE user_id = $1 AND state = 'failed';`, u.ID).Scan(&count); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}

func (u *User) CanLogin() bool {
	if u.ServiceAccount {
		return true
	}
	if u.ConfirmedAt.Valid {
		return true
	}
	if u.CreatedAt != nil && u.CreatedAt.Add(maxUnconfirmedDuration).After(time.Now()) {
		return true
	}
	return false
}

func (u *User) CheckCanUseDesktopApp(d *db.Database) (bool, error) {
	if u.CanUseDesktopApp.Bool {
		return true, nil
	}

	bp, err := u.BillingProfile(d)
	if err != nil {
		return false, err
	}

	for _, state := range AllowedDesktopAppBillingStates {
		if bp.State == state {
			return true, nil
		}
	}

	// now check org memberships
	arg := map[string]interface{}{
		"user_id": u.ID,
		"states":  AllowedDesktopAppBillingStates,
	}
	query, args, err := sqlx.Named(`SELECT COUNT(b.id) = 1 FROM billing_profiles b LEFT JOIN org_memberships o ON o.org_id = b.org_id WHERE o.user_id = :user_id AND b.state IN (:states) LIMIT 1;`, arg)
	if err != nil {
		return false, err
	}
	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return false, err
	}
	query = d.Rebind(query)
	var success bool
	err = d.Get(&success, query, args...)
	if err != nil {
		return false, err
	}
	return success, nil
}

func (u *User) ConfirmationTokenExpired() bool {
	if !u.ConfirmationSentAt.Valid {
		return true
	}
	return u.ConfirmationSentAt.Time.Add(maxConfirmationHours).Before(time.Now())
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
	return nstmt.QueryRowx(u).StructScan(u)
}

func (u *User) ClearResetPasswordToken(d db.QueryReceiver) error {
	return d.QueryRowx("UPDATE users SET reset_password_token = NULL WHERE id = $1 RETURNING *;", u.ID).StructScan(u)
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

func (u *User) GenerateConfirmationURL(d db.QueryReceiver) (*url.URL, error) {
	t, err := securerandom.Base64(15)
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("%s%d", t[:20], u.ID)
	u.ConfirmationToken = sql.NullString{token, true}

	nstmt, err := d.PrepareNamed(`UPDATE users SET (confirmation_token, confirmation_sent_at) = (:confirmation_token, now()) WHERE id = :id RETURNING *;`)
	if err != nil {
		return nil, err
	}
	err = nstmt.QueryRowx(u).StructScan(u)
	if err != nil {
		return nil, err
	}

	host := envvars.WebHostname
	if host == "" {
		host = "www.nitrous.io"
	}
	return &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app/",
		Fragment: fmt.Sprintf("/confirm-email?token=%s", u.ConfirmationToken.String),
	}, nil
}

func (u *User) SendUsageNoticeEmail(d db.QueryReceiver, hour int64) error {
	var hoursRemaining string
	if hour > 1 {
		hoursRemaining = strconv.FormatInt(hour, 10) + " hours"
	} else {
		hoursRemaining = strconv.FormatInt(hour, 10) + " hour"
	}
	usageQuotaInHour := u.UsageQuota.Int64 / 60 / 60
	hoursUsed := usageQuotaInHour - hour
	usageQuotaNextRestoredAt := u.UsageQuotaRestoredAt.Time.Add(time.Duration(u.UsageQuotaPeriod.Int64) * time.Second)
	tmplData := struct {
		Name                     string
		HoursRemaining           string
		HoursUsed                int64
		UsageQuotaNextRestoredAt string
		UsageQuotaInHour         int64
		UsageQuotaPeriod         int64
	}{
		u.DisplayName(),
		hoursRemaining,
		hoursUsed,
		usageQuotaNextRestoredAt.Format("2 Jan 3:04PM GMT"),
		usageQuotaInHour,
		u.UsageQuotaPeriod.Int64 / 60 / 60 / 24,
	}

	htmlTmpl := usageNoticeEmailHtmlTemplate
	txtTmpl := usageNoticeEmailTxtTemplate
	subject := fmt.Sprintf("Your free Nitrous workstation: %s remaining this month", hoursRemaining)
	from := envvars.SupportEmail
	to := (&mail.Address{Name: u.DisplayName(), Address: u.Email}).String()

	htmlBuf := bytes.NewBuffer(nil)
	if err := htmlTmpl.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err := txtTmpl.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	return Mailer.SendMail(from, []string{to}, []string{from}, nil, from, subject, txtBuf.String(), htmlBuf.String())
}

func (u *User) SendConfirmationEmail(d db.QueryReceiver, newUser bool) error {
	resetURL, err := u.GenerateConfirmationURL(d)
	if err != nil {
		return err
	}

	tmplData := struct {
		Name       string
		ConfirmUrl string
	}{
		u.DisplayName(),
		resetURL.String(),
	}

	htmlTmpl := confirmEmailHtmlTemplate
	txtTmpl := confirmEmailTxtTemplate
	subject := "Confirm your Nitrous email"

	if newUser {
		htmlTmpl = confirmEmailNewUserHtmlTemplate
		txtTmpl = confirmEmailNewUserTxtTemplate
	}

	// send email
	htmlBuf := bytes.NewBuffer(nil)
	if err := htmlTmpl.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err := txtTmpl.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	var targetEmail string
	if u.UnconfirmedEmail.String != "" {
		targetEmail = u.UnconfirmedEmail.String
	} else {
		targetEmail = u.Email
	}

	from := envvars.SupportEmail
	to := (&mail.Address{Name: u.DisplayName(), Address: targetEmail}).String()

	return Mailer.SendMail(from, []string{to}, []string{from}, nil, from, subject, txtBuf.String(), htmlBuf.String())
}

func (u *User) ConfirmEmail(d db.QueryReceiver) error {
	if !u.ConfirmationSentAt.Valid {
		return ErrConfirmationAlreadyCompleted
	}

	if u.ConfirmationTokenExpired() {
		return ErrConfirmationLapsed
	}

	previousEmail := ""
	if u.UnconfirmedEmail.String != "" {
		previousEmail = u.Email
		u.Email = u.UnconfirmedEmail.String
		u.UnconfirmedEmail.String = ""
	}

	nstmt, err := d.PrepareNamed(`UPDATE users
		SET (email, unconfirmed_email, confirmation_token, confirmed_at, confirmation_sent_at)
		= (:email, NULL, NULL, now(), NULL)
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	if err := nstmt.QueryRowx(u).StructScan(u); err != nil {
		return err
	}

	go func() {
		if err := u.updateMailSubscriptions(d, u.Email, previousEmail); err != nil {
			logrus.Errorf("Error called updateMailSubscriptions for user %d, err: %v", u.ID, err)
		}
	}()

	// update user e-mail in Stripe so their invoices go to new address
	if previousEmail != "" {
		bp, err := u.BillingProfile(d)
		if err != nil {
			return err
		}
		go func() {
			if err := bp.UpdateEmail(d, u.Email); err != nil {
				logrus.Errorf("ConfirmEmail: error updating e-mail on stripe for user %s to %s", u.Username, u.Email)
			}
		}()
	}

	_, err = d.Exec("UPDATE container_collabs SET user_id = $1, invite_email = NULL, invite_code = NULL WHERE invite_email = $2", u.ID, u.Email)
	return err
}

func (u *User) updateMailSubscriptions(d db.QueryReceiver, email, previousEmail string) error {
	profile, err := userprofile.Find(d, u.ID)
	if err != nil {
		return err
	}

	listIDs := []string{}
	if profile.SubscribeNewsletter {
		listIDs = append(listIDs, envvars.MailChimpIDNewsletter)
	}
	if profile.SubscribeAnnouncements {
		listIDs = append(listIDs, envvars.MailChimpIDAnnouncements)
	}

	for _, id := range listIDs {
		if previousEmail != "" {
			if err := MailChimp.Unsubscribe(id, previousEmail); err != nil {
				logrus.Errorf("MailChimp unsubscribe failed for user %d, err: %v", u.ID, err)
			}
		}
		opts := &mailchimp.SubscribeOptions{
			Email: email,
			MergeFields: map[string]interface{}{
				"FNAME":    u.DisplayName(),
				"USERNAME": u.Username,
			},
		}
		if err := MailChimp.Subscribe(id, opts); err != nil {
			logrus.Errorf("MailChimp subscribe failed for user %d, err: %v", u.ID, err)
		}
	}
	return nil
}

func (u *User) SendPasswordResetEmail(d db.QueryReceiver) error {
	err := u.SetResetPasswordToken(d)
	if err != nil {
		return err
	}

	from := envvars.SupportEmail
	to := (&mail.Address{Name: u.DisplayName(), Address: u.Email}).String()
	replyTo := from
	subject := fmt.Sprintf("[nitrous-pro] Reset password instructions")

	host := envvars.WebHostname
	if host == "" {
		host = "www.nitrous.io"
	}
	resetURL := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app",
		Fragment: fmt.Sprintf("/reset-password?token=%s", u.ResetPasswordToken.String),
	}
	params := map[string]string{"Name": u.DisplayName(), "URL": resetURL.String()}

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
func (u *User) ActivateBillingProfile(d *db.Database, token string) (bp *billingprofile.BillingProfile, err error) {
	params := &stripe.CustomerParams{
		Email: u.Email,
		Desc:  fmt.Sprintf("Name: %s, Username: %s", u.Name, u.Username),
		Params: stripe.Params{
			Meta: map[string]string{
				"username": u.Username,
				"name":     u.Name,
			},
		},
	}
	if token != "" {
		params.Token = token
	}
	c, err := customer.New(params)
	if err != nil {
		err = billingprofile.HandleStripeError(err)
		return
	}

	// remove the Stripe profile to prevent junk in the Stripe dashboard
	defer func() {
		if err != nil {
			if err := customer.Del(c.ID); err != nil {
				logrus.Errorf("Unable to delete customer %s, err: %v", c.ID, err)
			}
		}
	}()

	bp, err = u.BillingProfile(d)
	if err != nil {
		return
	}

	if err = bp.Activate(d, c); err != nil {
		if err == billingprofile.ErrCardBanned {
			err = ErrCardBanned
		}
		return
	}

	err = nil
	return
}

// populates Suspicious field if conditions are matched
func (u *User) CheckSuspicious() {
	reasons := []string{}
	if u.Suspicious.Valid {
		reasons = append(reasons, u.Suspicious.String)
	}

	// known abusers
	// TODO move to config file
	signatures := []string{
		"viros",
		"aoqui",
		"mafia",
		"grimm",
		"resende",
		"slk123",
		"pekeno",
		"p3k3n0",
		"pekenino",
		"wavee",
		"governadorr",
	}

	for _, sig := range signatures {
		if strings.Contains(u.Username, sig) {
			reasons = append(reasons, fmt.Sprintf("Signature match username: %s", sig))
		}
		if strings.Contains(u.Email, sig) {
			reasons = append(reasons, fmt.Sprintf("Signature match email: %s", sig))
		}
	}

	if len(reasons) == 0 {
		return
	}

	u.Suspicious.Valid = true
	u.Suspicious.String = strings.Join(reasons, "; ")
}

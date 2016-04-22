package user

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
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
	UsageQuotaRestoredAt pq.NullTime   `db:"usage_quota_restored_at"`
	UsageNoticesSent     sql.NullInt64 `db:"usage_notices_sent"`

	CreatedByClientID sql.NullInt64 `db:"created_by_client_id"`

	// cached billing profile
	billingProfile *billingprofile.BillingProfile

	// deprecated
	PreviousPromoCode sql.NullString `db:"previous_promo_code"`
}

func (u *User) GrantBonus(d *db.Database, bn *bonus.Bonus) error {
	// We want to rollback transaction if user has already earned bonus
	return d.WithTransaction(func(tx *sqlx.Tx) error {
		err := tx.QueryRowx(`UPDATE users
		SET
		  usage_quota = usage_quota + $1,
                 updated_at = now()
               FROM hosts
               WHERE (
                 users.id = $2
                 AND hosts.user_id = users.id
                 AND hosts.free = true
                 AND hosts.terminated_at IS NULL
               )
               RETURNING (users.*);
		`, bn.Amount, u.ID).StructScan(u)
		if err != nil {
			return err
		}

		ebn := &bonus.EarnedBonus{
			BonusID: bn.ID,
			UserID:  u.ID,
		}
		return ebn.Insert(tx)
	})
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

// RestoreUsageQuotas restores the usage_quota_restored_at and usage_notices_sent
// for all users who:
// 1. have a free host, and
// 2. had their free usage quota last restored earlier than their usage quota
//    period.
func RestoreUsageQuotas(d db.QueryReceiver, when time.Time) (int64, error) {
	res, err := d.Exec(`UPDATE users
		SET
			usage_quota_restored_at = now(),
			usage_notices_sent = 0
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

func FindAllWithFreeHost(d db.QueryReceiver) ([]*User, error) {
	users := []*User{}
	err := d.Select(&users, `
		SELECT DISTINCT(users.*) FROM users
			JOIN hosts on hosts.user_id = users.id
		WHERE
			hosts.free = true
			AND hosts.terminated_at IS NULL;
	`)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func FindAllByNoticeSent(d db.QueryReceiver, noticeSent int64) ([]*User, error) {
	users := []*User{}
	err := d.Select(&users, `
		SELECT u.* FROM users u
			INNER JOIN hosts AS h ON h.user_id = u.id
		WHERE h.free = true
			AND h.terminated_at IS NULL
			AND u.usage_notices_sent = $1
		ORDER BY u.id;
	`, noticeSent)
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
			usage_quota_restored_at, usage_notices_sent,
			updated_at)
		= (:name, :email, :encrypted_password,
			:unconfirmed_email, :confirmation_sent_at, :confirmation_token,
			:usage_quota_restored_at, :usage_notices_sent,
			now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(u).StructScan(u)
}

// There are some host_transitions with running state and null ended_at.
// Before compute free host usage, we have to clean up bad host_transitions row of a user.
// We do it by filling up ended_at with created_at of the next host_transition.
func (u *User) FreeHostUsage(db *db.Database, endTime time.Time) (int64, error) {
	type HostTransition struct {
		ID            int64          `db:"id"`
		HostID        int64          `db:"host_id"`
		InitiatorID   sql.NullInt64  `db:"initiator_id"`
		PreviousState sql.NullString `db:"previous_state"`
		State         string         `db:"state"`
		CreatedAt     *time.Time     `db:"created_at"`
		EndedAt       *time.Time     `db:"ended_at"`
		Metadata      hstore.Hstore  `db:"metadata"`
	}

	err := db.WithTransaction(func(tx *sqlx.Tx) error {
		// select all host transitions of user's running, free hosts
		// which ended_at are null and omit the last row of each host
		hostTransitions := []*HostTransition{}
		err := tx.Select(&hostTransitions, `
			SELECT ht.*
			FROM host_transitions AS ht
				JOIN hosts AS h on ht.host_id = h.id
				JOIN users AS u on h.user_id = u.id
			WHERE u.id = $1
				AND ht.state = 'running'
				AND h.free = true
				AND ht.ended_at IS NULL
				AND ht.id < (
					SELECT MAX(ht1.id) FROM host_transitions ht1
					JOIN hosts AS h1 ON ht1.host_id = h1.id
					JOIN users AS u1 ON u1.id = h1.user_id
					WHERE u.id = $1
					AND ht1.host_id = ht.host_id
				)
		`, u.ID)
		if err != nil {
			if err != sql.ErrNoRows {
				return err
			}
		}

		for _, ht := range hostTransitions {
			nextOfNullEndedAtHostTransitions := &HostTransition{}
			err = tx.Get(nextOfNullEndedAtHostTransitions, `
			SELECT * FROM host_transitions
			WHERE host_id = $1
			AND created_at > $2
			ORDER BY created_at
			LIMIT 1
		`, ht.HostID, ht.CreatedAt)
			if err != nil {
				if err == sql.ErrNoRows {
					continue
				} else {
					return err
				}
			}

			newEndedAt := nextOfNullEndedAtHostTransitions.CreatedAt
			_, err = tx.Exec(`
				UPDATE host_transitions SET ended_at = $1 WHERE id = $2
			`, newEndedAt, ht.ID)
			if err != nil {
				return err
			}

			logrus.WithFields(logrus.Fields{
				"alertdevs": true,
				"host_id":   ht.HostID,
				"ht_id":     ht.ID,
			}).Warnf("host_transitions: null ended_at with running state for id: %d", ht.ID)
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	var seconds int64
	err = db.QueryRowx(`
		SELECT COALESCE(SUM(
			CAST(EXTRACT(
				EPOCH FROM (
					COALESCE(ht.ended_at, $1) - GREATEST(ht.created_at, u.usage_quota_restored_at)
				)
			) AS INTEGER)
		), 0)
		FROM host_transitions AS ht
			JOIN hosts AS h on ht.host_id = h.id
			JOIN users AS u on h.user_id = u.id
		WHERE u.id = $2
			AND ht.state = 'running'
			AND h.free = true
			AND (ht.ended_at > u.usage_quota_restored_at OR ht.ended_at IS NULL);
	`, endTime, u.ID).Scan(&seconds)
	if err != nil {
		return 0, err
	}
	return seconds, nil
}

func (u *User) HasUsageRemaining(db *db.Database) bool {
	seconds, err := u.FreeHostUsage(db, time.Now())
	if err != nil {
		return false
	}
	return seconds < u.UsageQuota.Int64
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

func (u *User) SendUsageNoticeEmail(db *db.Database) error {
	var usageQuotaInHours int64
	if u.UsageQuota.Valid {
		usageQuotaInHours = u.UsageQuota.Int64 / 60 / 60
	}

	secondsUsed, err := u.FreeHostUsage(db, time.Now())
	if err != nil {
		return err
	}

	hoursUsed := int64(math.Ceil(float64(secondsUsed) / 60 / 60))
	if hoursUsed > usageQuotaInHours {
		hoursUsed = usageQuotaInHours
	}

	var usageQuotaNextRestoredAt time.Time
	if u.UsageQuotaRestoredAt.Valid {
		usageQuotaNextRestoredAt = u.UsageQuotaRestoredAt.Time.Add(time.Duration(u.UsageQuotaPeriod.Int64) * time.Second)
	}

	hoursRemaining := usageQuotaInHours - hoursUsed
	var hoursRemainingStr string
	if hoursRemaining <= 1 {
		hoursRemainingStr = fmt.Sprintf("%d hour", hoursRemaining)
	} else {
		hoursRemainingStr = fmt.Sprintf("%d hours", hoursRemaining)
	}

	tmplData := struct {
		Name                     string
		HoursRemaining           string
		HoursUsed                int64
		UsageQuotaNextRestoredAt string
		UsageQuotaInHour         int64
		UsageQuotaPeriod         int64
	}{
		u.DisplayName(),
		hoursRemainingStr,
		hoursUsed,
		usageQuotaNextRestoredAt.Format("2 Jan 3:04PM GMT"),
		usageQuotaInHours,
		u.UsageQuotaPeriod.Int64 / 60 / 60 / 24,
	}

	subject := fmt.Sprintf("Your free Nitrous workstation: %s remaining this month", hoursRemainingStr)
	from := envvars.SupportEmail
	to := (&mail.Address{Name: u.DisplayName(), Address: u.Email}).String()

	htmlBuf := bytes.NewBuffer(nil)
	if err := usageNoticeEmailHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err := usageNoticeEmailTxtTemplate.Execute(txtBuf, tmplData); err != nil {
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
	if u.ConfirmedAt.Valid {
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
func (u *User) ActivateBillingProfile(d *db.Database, token, ip string) (*billingprofile.BillingProfile, error) {
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

		params.SetSource(&stripe.CardParams{
			Token: token,
		})
	}
	c, err := customer.New(params)
	if err != nil {
		return nil, billingprofile.HandleStripeError(err)
	}

	// remove the Stripe profile to prevent junk in the Stripe dashboard
	defer func() {
		if err != nil {
			if _, err := customer.Del(c.ID); err != nil {
				logrus.Errorf("Unable to delete customer %s, err: %v", c.ID, err)
			}
		}
	}()

	// Check that the card country and ip address country match, to prevent fraud
	if c.Sources != nil && len(c.Sources.Values) != 0 {
		if !billingprofile.CardCountryMatch(ip, c.Sources.Values[0].Card.CardCountry) {
			return nil, billingprofile.ErrCardCountryMismatch
		}
	}

	bp, err := u.BillingProfile(d)
	if err != nil {
		return nil, err
	}

	err = bp.Activate(d, c)
	return bp, err
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

package engagement

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"net/url"
	"strconv"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
)

var (
	Mailer mailer.Mailer

	ActivationEmailInterval time.Duration

	GetStartedEmailIntervalFirst  time.Duration
	GetStartedEmailIntervalSecond time.Duration
	GetStartedEmailIntervalThird  time.Duration
)

var (
	ErrActivationEmailAlreadySent   = errors.New("The user has already had an activation email sent")
	ErrGetStartedEmailNotApplicable = errors.New("The get started email is not applicable to this user")
)

func extractDuration(val, defaultVal string) time.Duration {
	if val == "" {
		val = defaultVal
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		log.Fatal(err)
	}
	return time.Hour * time.Duration(parsed)
}

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)

	ActivationEmailInterval = extractDuration(envvars.ActivationEmailInterval, "12")

	GetStartedEmailIntervalFirst = extractDuration(envvars.GetStartedEmailIntervalFirst, "24")
	GetStartedEmailIntervalSecond = extractDuration(envvars.GetStartedEmailIntervalSecond, "48")
	GetStartedEmailIntervalThird = extractDuration(envvars.GetStartedEmailIntervalThird, "120")
}

func ActivationEmailCandidates(d *db.Database) ([]user.User, error) {
	users := []user.User{}
	query := `SELECT users.* FROM users
						INNER JOIN user_profiles ON user_profiles.user_id = users.id
						INNER JOIN billing_profiles ON billing_profiles.user_id = users.id
						WHERE users.confirmation_token IS NULL
						AND	users.service_account IS FALSE
						AND users.created_at <= now() - interval '%d milliseconds'
						AND user_profiles.activation_email_sent_at IS NULL
						AND billing_profiles.state = $1;`
	parsedQuery := fmt.Sprintf(query, ActivationEmailInterval/time.Millisecond)
	err := d.Select(&users, parsedQuery, billingprofile.StateNoCard)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func SendActivationEmail(d *db.Database, u user.User) (err error) {
	host := envvars.WebHostname
	if host == "" {
		host = "pro.nitrous.io"
	}
	actURL := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app",
		Fragment: "billing",
	}

	tmplData := struct {
		Name        string
		ActivateUrl string
	}{
		u.DisplayName(),
		actURL.String(),
	}

	tx, err := d.Beginx()
	if err != nil {
		return err
	}

	defer func() {
		if err == nil {
			return
		}

		tx.Rollback()
	}()

	var ret struct {
		UserID sql.NullInt64 `db:"user_id"`
	}
	err = tx.QueryRowx(`UPDATE user_profiles SET activation_email_sent_at = now() WHERE user_id = $1 AND activation_email_sent_at IS NULL RETURNING user_id;`, u.ID).StructScan(&ret)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrActivationEmailAlreadySent
		}
		return err
	}
	if !ret.UserID.Valid {
		return ErrActivationEmailAlreadySent
	}

	// send email
	htmlBuf := bytes.NewBuffer(nil)
	if err = activateEmailHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err = activateEmailTxtTemplate.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	var targetEmail string
	if u.UnconfirmedEmail.String != "" {
		targetEmail = u.UnconfirmedEmail.String
	} else {
		targetEmail = u.Email
	}

	from := (&mail.Address{Name: "Nitrous Pro", Address: "mailer@nitrous.io"}).String()
	to := (&mail.Address{Name: tmplData.Name, Address: targetEmail}).String()

	err = Mailer.SendMail(from, []string{to}, []string{from}, nil, from, "Activate your Nitrous account", txtBuf.String(), htmlBuf.String())
	if err != nil {
		return err
	}

	return tx.Commit()
}

func GetStartedEmailCandidates(d *db.Database) ([]user.User, error) {
	users := []user.User{}
	query := `SELECT users.* FROM users
						INNER JOIN user_profiles ON user_profiles.user_id = users.id
						INNER JOIN billing_profiles ON billing_profiles.user_id = users.id
						WHERE billing_profiles.state = $1
						AND	users.service_account IS FALSE
						AND (user_profiles.activation_email_sent_at IS NULL OR user_profiles.activation_email_sent_at <= now() - interval '%d milliseconds')
						AND (
							(users.created_at <= now() - interval '%d milliseconds' AND user_profiles.starter_credit_email_send_count = 0)
							OR (user_profiles.starter_credit_email_last_sent_at <= now() - interval '%d milliseconds' AND user_profiles.starter_credit_email_send_count = 1)
							OR (user_profiles.starter_credit_email_last_sent_at <= now() - interval '%d milliseconds' AND user_profiles.starter_credit_email_send_count = 2)
						);`
	parsedQuery := fmt.Sprintf(query, ActivationEmailInterval/time.Millisecond, GetStartedEmailIntervalFirst/time.Millisecond, GetStartedEmailIntervalSecond/time.Millisecond, GetStartedEmailIntervalThird/time.Millisecond)
	err := d.Select(&users, parsedQuery, billingprofile.StateNoCard)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func SendGetStartedEmail(d *db.Database, u user.User) (err error) {
	tx, err := d.Beginx()
	if err != nil {
		return err
	}

	defer func() {
		if err == nil {
			return
		}

		tx.Rollback()
	}()

	var ret struct {
		UserID sql.NullInt64 `db:"user_id"`
	}
	query := `UPDATE user_profiles
						SET starter_credit_email_send_count = starter_credit_email_send_count + 1,
						starter_credit_email_last_sent_at = now()
						WHERE user_id = $1
						AND (activation_email_sent_at IS NULL OR activation_email_sent_at <= now() - interval '%d milliseconds')
						AND (
							(created_at <= now() - interval '%d milliseconds' AND starter_credit_email_send_count = 0)
							OR (starter_credit_email_last_sent_at <= now() - interval '%d milliseconds' AND starter_credit_email_send_count = 1)
							OR (starter_credit_email_last_sent_at <= now() - interval '%d milliseconds' AND starter_credit_email_send_count = 2)
						) RETURNING user_id`
	parsedQuery := fmt.Sprintf(query, ActivationEmailInterval/time.Millisecond, GetStartedEmailIntervalFirst/time.Millisecond, GetStartedEmailIntervalSecond/time.Millisecond, GetStartedEmailIntervalThird/time.Millisecond)
	err = tx.QueryRowx(parsedQuery, u.ID).StructScan(&ret)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrGetStartedEmailNotApplicable
		}
		return err
	}
	if !ret.UserID.Valid {
		return ErrGetStartedEmailNotApplicable
	}

	var claimUrl *url.URL
	if !u.ConfirmedAt.Valid && u.ConfirmationTokenExpired() {
		// refresh the confirmation token
		claimUrl, err = u.GenerateConfirmationURL(tx)
		if err != nil {
			return err
		}
	}

	if claimUrl == nil {
		host := envvars.WebHostname
		if host == "" {
			host = "pro.nitrous.io"
		}
		claimUrl = &url.URL{
			Scheme: "https",
			Host:   host,
			Path:   "/app",
		}
	}

	tmplData := struct {
		Name     string
		ClaimUrl string
	}{
		u.DisplayName(),
		claimUrl.String(),
	}

	// send email
	htmlBuf := bytes.NewBuffer(nil)
	if err = getStartedEmailHtmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err = getStartedEmailTxtTemplate.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	var targetEmail string
	if u.UnconfirmedEmail.String != "" {
		targetEmail = u.UnconfirmedEmail.String
	} else {
		targetEmail = u.Email
	}

	from := (&mail.Address{Name: "Nitrous Pro", Address: "mailer@nitrous.io"}).String()
	to := (&mail.Address{Name: tmplData.Name, Address: targetEmail}).String()

	err = Mailer.SendMail(from, []string{to}, []string{from}, nil, from, "Nitrous: $5 Credit to Get Started", txtBuf.String(), htmlBuf.String())
	if err != nil {
		return err
	}

	return tx.Commit()
}

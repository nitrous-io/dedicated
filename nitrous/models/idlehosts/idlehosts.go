package idlehosts

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"text/template"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	stripe "github.com/stripe/stripe-go"
	stripesub "github.com/stripe/stripe-go/sub"
)

var (
	IdleTimeout                  = 14 * 24 * time.Hour
	FirstWarningTimeout          = IdleTimeout - (7 * 24 * time.Hour)
	NextCheckAfterFirstWarning   = FirstWarningTimeout - (24 * time.Hour) // after first warning is sent, second warning occurs 7 days - 24hrs
	TerminationAfterFinalWarning = 24 * time.Hour
	Mailer                       mailer.Mailer
)

var (
	ErrLockNotObtained              = errors.New("Lock was not obtained")
	ErrLastRunningAtInvalid         = errors.New("LastRunningAt is not valid")
	ErrHostStartedWithinIdleTimeout = errors.New("Host has been started since IdleTimeout")
)

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)

	total := FirstWarningTimeout + NextCheckAfterFirstWarning + TerminationAfterFinalWarning
	if total != IdleTimeout {
		logrus.Fatalf("IdleTimeout should add up to all Warning checks", total, IdleTimeout)
	}
}

type Target struct {
	UserName      string         `db:"user_name"`
	UserUsername  string         `db:"user_username"`
	UserEmail     string         `db:"user_email"`
	UserDeletedAt pq.NullTime    `db:"user_deleted_at"`
	UserID        int64          `db:"user_id"`
	HostID        int64          `db:"host_id"`
	HostSlug      string         `db:"host_slug"`
	HostName      sql.NullString `db:"host_name"`
	SubSlug       string         `db:"sub_slug"`
}

func (t *Target) UserDisplayName() string {
	name := t.UserName
	if name == "" {
		name = t.UserUsername
	}
	return name
}

type warningOpts struct {
	noticeNumber int
	nextCheck    time.Duration
	subject      string
	htmlTemplate *template.Template
	txtTemplate  *template.Template
}

func (t *Target) SendFirstWarning(db *ndb.Database) error {
	opts := &warningOpts{
		noticeNumber: host.IDLE_NOTICES_FIRST_WARNING,
		nextCheck:    NextCheckAfterFirstWarning,
		subject:      fmt.Sprintf("Nitrous - your workspace '%s' will be deleted in 1 week", t.HostName.String),
		htmlTemplate: firstWarningHtmlTemplate,
		txtTemplate:  firstWarningTxtTemplate,
	}

	return t.sendWarning(db, opts)
}

func (t *Target) SendFinalWarning(db *ndb.Database) error {
	opts := &warningOpts{
		noticeNumber: host.IDLE_NOTICES_FINAL_WARNING,
		nextCheck:    TerminationAfterFinalWarning,
		subject:      fmt.Sprintf("Nitrous - your workspace '%s' will be deleted in 24 hours", t.HostName.String),
		htmlTemplate: finalWarningHtmlTemplate,
		txtTemplate:  finalWarningTxtTemplate,
	}

	return t.sendWarning(db, opts)
}

func (t *Target) Terminate(db *ndb.Database) error {
	var data struct {
		LastRunningAt   pq.NullTime    `db:"last_running_at"`
		IdleNoticesSent int            `db:"idle_notices_sent"`
		BPStripeID      sql.NullString `db:"bp_stripe_id"`
		SubStripeID     sql.NullString `db:"sub_stripe_id"`
	}
	err := db.Get(&data,
		`SELECT hosts.last_running_at, hosts.idle_notices_sent,
                        bp.stripe_id AS bp_stripe_id, sub.stripe_id AS sub_stripe_id
                FROM hosts
                INNER JOIN subscriptions sub ON hosts.subscription_id = sub.id
                INNER JOIN plans ON plans.id = hosts.plan_id
                INNER JOIN billing_profiles bp ON sub.billing_profile_id = bp.id
                WHERE hosts.id = $1
                AND plans.price = 0
                AND sub.cancel_pending IS FALSE
                AND sub.cancelled_at IS NULL
                AND sub.state != $2
                AND hosts.terminated_at IS NULL;`, t.HostID, subscription.StateDeleted)
	if err != nil {
		return err
	}

	if data.IdleNoticesSent != host.IDLE_NOTICES_FINAL_WARNING {
		return fmt.Errorf("Expected idle_notices_sent to equal 2, got %d", data.IdleNoticesSent)
	}

	if !data.LastRunningAt.Valid {
		return ErrLastRunningAtInvalid
	}

	if data.LastRunningAt.Time.After(time.Now().Add(-1 * IdleTimeout)) {
		return ErrHostStartedWithinIdleTimeout
	}

	// create advisory lock
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	obtained, err := ndb.TransactionLock(tx, ndb.IDLE_HOST_LOCK, int(t.HostID))
	if err != nil {
		return err
	}
	if !obtained {
		return ErrLockNotObtained
	}

	return stripesub.Cancel(
		data.SubStripeID.String,
		&stripe.SubParams{Customer: data.BPStripeID.String, EndCancel: false},
	)
}

func FirstWarningTargets(db *ndb.Database) ([]*Target, error) {
	targets := []*Target{}
	query := fmt.Sprintf(`SELECT
                users.name AS user_name,
                users.username AS user_username,
                users.email AS user_email,
                users.id AS user_id,
                users.deleted_at AS user_deleted_at,
                hosts.id AS host_id,
                hosts.slug AS host_slug,
                hosts.name AS host_name,
                sub.slug AS sub_slug
                FROM hosts
                INNER JOIN plans ON plans.id = hosts.plan_id
                INNER JOIN users ON users.id = hosts.user_id
                INNER JOIN subscriptions sub ON sub.id = hosts.subscription_id
                WHERE plans.price = 0
                AND hosts.terminated_at IS NULL
                AND hosts.idle_notices_sent = $1
                AND GREATEST(last_running_at, last_pinged_at) <= now() - interval '%d seconds';`,
		int64(FirstWarningTimeout/time.Second))
	err := db.Select(&targets, query, host.IDLE_NOTICES_NO_WARNING)
	if err != nil {
		return nil, err
	}
	return targets, nil
}

func FinalWarningTargets(db *ndb.Database) ([]*Target, error) {
	targets := []*Target{}
	err := db.Select(&targets, `SELECT
                users.name AS user_name,
                users.username AS user_username,
                users.email AS user_email,
                users.id AS user_id,
                users.deleted_at AS user_deleted_at,
                hosts.id AS host_id,
                hosts.slug AS host_slug,
                hosts.name AS host_name,
                sub.slug AS sub_slug
                FROM hosts
                INNER JOIN plans ON plans.id = hosts.plan_id
                INNER JOIN users ON users.id = hosts.user_id
                INNER JOIN subscriptions sub ON sub.id = hosts.subscription_id
                WHERE plans.price = 0
                AND hosts.terminated_at IS NULL
                AND hosts.idle_notices_sent = $1
                AND hosts.idle_next_check <= now();`,
		host.IDLE_NOTICES_FIRST_WARNING)
	if err != nil {
		return nil, err
	}
	return targets, nil
}

func TerminationTargets(db *ndb.Database) ([]*Target, error) {
	targets := []*Target{}
	query := fmt.Sprintf(`SELECT
                users.name AS user_name,
                users.username AS user_username,
                users.email AS user_email,
                users.id AS user_id,
                users.deleted_at AS user_deleted_at,
                hosts.id AS host_id,
                hosts.slug AS host_slug,
                hosts.name AS host_name
                FROM hosts
                INNER JOIN plans ON plans.id = hosts.plan_id
                INNER JOIN users ON users.id = hosts.user_id
                WHERE plans.price = 0
                AND hosts.terminated_at IS NULL
                AND hosts.idle_notices_sent = $1
                AND hosts.idle_next_check <= now()
                AND GREATEST(last_running_at, last_pinged_at) <= now() - interval '%d seconds';`,
		int64(IdleTimeout/time.Second))
	err := db.Select(&targets, query, host.IDLE_NOTICES_FINAL_WARNING)
	if err != nil {
		return nil, err
	}
	return targets, nil
}

func (t *Target) sendWarning(db *ndb.Database, opt *warningOpts) error {
	// ensure data is no longer stale
	var noticesSent int
	err := db.QueryRow(`SELECT idle_notices_sent FROM hosts WHERE id = $1;`, t.HostID).Scan(&noticesSent)
	if err != nil {
		return err
	}
	if noticesSent != (opt.noticeNumber - 1) {
		err = fmt.Errorf("Expected idle_notices_sent to equal %d, got %d", opt.noticeNumber-1, noticesSent)
		return err
	}

	// create Tx for advisory lock
	var tx *sqlx.Tx
	tx, err = db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var obtained bool
	obtained, err = ndb.TransactionLock(tx, ndb.IDLE_HOST_LOCK, int(t.HostID))
	if err != nil {
		return err
	}
	if !obtained {
		return ErrLockNotObtained
	}

	from := envvars.SupportEmailAddr
	to := &mail.Address{Name: t.UserDisplayName(), Address: t.UserEmail}

	host := envvars.WebHostname
	if host == "" {
		host = "www.nitrous.io"
	}
	dashboardUrl := &url.URL{
		Scheme: "https",
		Host:   host,
		Path:   "/app/",
	}
	upgradeUrl := &url.URL{
		Scheme:   "https",
		Host:     host,
		Path:     "/app/",
		Fragment: fmt.Sprintf("/subscriptions/%s/upgrade", t.SubSlug),
	}

	tmplData := struct {
		Name         string
		HostName     string
		SupportEmail string
		DashboardUrl string
		UpgradeUrl   string
	}{
		t.UserDisplayName(),
		t.HostName.String,
		from.Address,
		dashboardUrl.String(),
		upgradeUrl.String(),
	}

	htmlTemplate := opt.htmlTemplate
	txtTemplate := opt.txtTemplate

	// send email
	htmlBuf := bytes.NewBuffer(nil)
	if err = htmlTemplate.Execute(htmlBuf, tmplData); err != nil {
		return err
	}
	txtBuf := bytes.NewBuffer(nil)
	if err = txtTemplate.Execute(txtBuf, tmplData); err != nil {
		return err
	}

	err = Mailer.SendMail(from.String(), []string{to.String()}, []string{from.String()}, nil, from.String(), opt.subject, txtBuf.String(), htmlBuf.String())
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`UPDATE hosts SET
                        idle_notices_sent = $1,
                        idle_next_check = now() + interval '%d seconds',
                        updated_at = now()
                        WHERE id = $2;`, int64(opt.nextCheck/time.Second))
	_, err = db.Exec(query, opt.noticeNumber, t.HostID)
	return err
}

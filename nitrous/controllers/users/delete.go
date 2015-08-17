package users

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jinzhu/now"
	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailchimp"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/card"
)

var (
	hostCreateInProgressErr = httperror.New(http.StatusConflict, "host_creation_in_progress", "you cannot delete your account while a workspace is being created")
	hostsNotTerminatedErr   = httperror.New(http.StatusBadRequest, "hosts_not_terminated", "you must terminate your workspaces before deleting your account")
	outstandingInvoicesErr  = httperror.New(http.StatusBadRequest, "outstanding_invoices", "your invoices must be paid before deleting your account")
	outstandingUsageErr     = httperror.New(http.StatusBadRequest, "outstanding_usage", "you have outstanding usage for the month, please delete your account after your invoice has been issued")
	deleteForbiddenErr      = httperror.New(http.StatusForbidden, "delete_forbidden", "You do not have permission to delete the user account")
)

var (
	MailChimp mailchimp.MailChimpProvider
)

func Delete(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if !permission.DeleteUser(env.Token, env.User) {
		return deleteForbiddenErr
	}

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := canDeleteUser(tx, env.User); err != nil {
		return err
	}

	locked, err := db.TransactionLock(tx, db.HOST_CREATE_LOCK, int(env.User.ID))
	if err != nil {
		return err
	}
	if !locked {
		return hostCreateInProgressErr
	}

	queries := []string{
		"DELETE FROM tokens WHERE user_id = $1",
		"DELETE FROM container_collabs WHERE user_id = $1",
		"DELETE FROM verifications WHERE user_id = $1;",
		"UPDATE users SET deleted_at = now() WHERE id = $1",
	}

	for _, query := range queries {
		if _, err := tx.Exec(query, env.User.ID); err != nil {
			return err
		}
	}

	bp, err := billingprofile.Find(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	profile, err := userprofile.Find(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE billing_profiles
		SET last_four = NULL,
		card_type = NULL,
		exp_month = NULL,
		exp_year = NULL,
		pending_activation = TRUE,
		max_hours = NULL,
		state = $1
		WHERE user_id = $2 AND org_id IS NULL`, billingprofile.StateNoCard, env.User.ID)
	if err != nil {
		return err
	}

	if bp.StripeID.Valid {
		iter := card.List(&stripe.CardListParams{Customer: bp.StripeID.String})
		for iter.Next() {
			c := iter.Card()
			if c == nil {
				break
			}

			if err := card.Del(c.ID, &stripe.CardParams{Customer: bp.StripeID.String}); err != nil {
				return err
			}
		}
		if err := iter.Err(); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// unsubscribe user from MailChimp newsletters
	go func() {
		if !env.User.ConfirmedAt.Valid && !env.User.UnconfirmedEmail.Valid {
			// user never confirmed original email
			return
		}

		listIDs := []string{}
		if profile.SubscribeNewsletter {
			listIDs = append(listIDs, envvars.MailChimpIDNewsletter)
		}
		if profile.SubscribeAnnouncements {
			listIDs = append(listIDs, envvars.MailChimpIDAnnouncements)
		}

		for _, id := range listIDs {

			if err := MailChimp.Unsubscribe(id, env.User.Email); err != nil {
				logrus.Errorf("MailChimp unsubscribe failed for user %d, err: %v", env.User.ID, err)
			}
		}
	}()

	env.Tracker.SetProfileProps("set", env.User.ID, map[string]interface{}{
		"deleted_date": time.Now(),
	})
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func CheckCanDelete(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	err := canDeleteUser(env.DB, env.User)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func canDeleteUser(d db.QueryReceiver, u *user.User) error {
	hosts, err := host.FindAllByUser(d, u.ID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if len(hosts) > 0 {
		return hostsNotTerminatedErr
	}

	var unpaidExists bool
	err = d.Get(&unpaidExists, `SELECT
		COUNT(*) >= 1
		FROM invoices
		WHERE (state = $1 OR (state = $2 AND covered_by IS NULL)) AND user_id = $3 LIMIT 1;`, invoice.StateUnpaid, invoice.StateDeferred, u.ID)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if unpaidExists {
		return outstandingInvoicesErr
	}

	start := now.BeginningOfMonth()
	end := time.Now()
	res, err := biller.InvoiceItemsForUser(d, u.ID, start, end)
	if err != nil {
		return err
	}

	if res.TotalCharge > 0 {
		availableCredits, err := credit.AvailableCredits(d, u.ID)
		if err != nil {
			return err
		}
		if availableCredits < res.TotalCharge {
			return outstandingUsageErr
		}
	}

	return nil
}

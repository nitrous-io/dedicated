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
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailchimp"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/card"
)

var (
	hostCreateInProgressErr = httperror.New(http.StatusConflict, "host_creation_in_progress", "you cannot delete your account while a workstation is being created")
	outstandingInvoicesErr  = httperror.New(http.StatusBadRequest, "outstanding_invoices", "your invoices must be paid before deleting your account")
	outstandingUsageErr     = httperror.New(http.StatusBadRequest, "outstanding_usage", "you have outstanding usage for the month, please delete your account after your invoice has been issued")
	deleteForbiddenErr      = httperror.New(http.StatusForbidden, "delete_forbidden", "You do not have permission to delete the user account")
)

var (
	subProvider subscriptions.Provider      = subscriptions.NewProvider()
	MailChimp   mailchimp.MailChimpProvider = &mailchimp.MailChimp{}
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

	// Disallow if a host is currently being created.
	locked, err := db.TransactionLock(tx, db.HOST_CREATE_LOCK, int(env.User.ID))
	if err != nil {
		return err
	}
	if !locked {
		return hostCreateInProgressErr
	}

	// Terminate all hosts.
	// TODO This should be done in a delayed fashion since it takes awhile to
	// terminate a host, e.g. by marking the host for termination and using a
	// cronjob to actually terminate the host.
	hosts, err := host.FindAllUnterminated(tx, env.User.ID)
	if err != nil {
		return err
	}
	for _, h := range hosts {
		if err := hostcontrol.Terminate(tx, h, hostbuilderHeaders(env), env.User.ID, h.State); err != nil {
			return err
		}
	}

	// Cancel all subscriptions.
	subs, err := subscription.FindAllActiveByUserID(tx, env.User.ID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, sub := range subs {
		if sub.AlreadyCanceled() {
			continue
		}

		if err := subProvider.Cancel(env.DB, sub.Subscription); err != nil {
			return err
		}
	}

	queries := []string{
		"DELETE FROM tokens WHERE user_id = $1",
		"DELETE FROM container_collabs WHERE user_id = $1",
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

	// Delete all the user's credit cards from Stripe.
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
			// User never confirmed any email address.
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
	if err := canDeleteUser(env.DB, env.User); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func canDeleteUser(d db.QueryReceiver, u *user.User) error {
	var unpaidInvoiceExists bool
	err := d.Get(&unpaidInvoiceExists, `SELECT
		COUNT(*) >= 1
		FROM invoices
		WHERE (state = $1 OR (state = $2 AND covered_by IS NULL)) AND user_id = $3 LIMIT 1;`, invoice.StateUnpaid, invoice.StateDeferred, u.ID)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if unpaidInvoiceExists {
		return outstandingInvoicesErr
	}

	start := now.BeginningOfMonth()
	res, err := biller.InvoiceItemsForUser(d, u.ID, start, time.Now())
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

func hostbuilderHeaders(env *middleware.Env) http.Header {
	h := http.Header{}
	h.Set(constants.NitrousUUIDHeader, env.UUID)
	return h
}

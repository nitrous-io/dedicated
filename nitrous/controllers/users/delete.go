package users

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/jinzhu/now"
	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/card"
)

var (
	hostCreateInProgressErr = httperror.New(http.StatusConflict, "host_creation_in_progress", "you cannot delete your account while a host is being created")
	hostsNotTerminatedErr   = httperror.New(http.StatusBadRequest, "hosts_not_terminated", "you must terminate your hosts before deleting your account")
	outstandingInvoicesErr  = httperror.New(http.StatusBadRequest, "outstanding_invoices", "your invoices must be paid before deleting your account")
	outstandingUsageErr     = httperror.New(http.StatusBadRequest, "outstanding_usage", "you have outstanding usage for the month, please delete your account after your invoice has been issued")
)

func Delete(env *middleware.Env, w http.ResponseWriter, r *http.Request) (err error) {
	tx, err := env.DB.Beginx()

	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		tx.Commit()
	}()

	err = canDeleteUser(tx, env.User)
	if err != nil {
		return err
	}

	locked, err := db.TransactionLock(tx, db.HOST_CREATE_LOCK, int(env.User.ID))
	if err != nil {
		return err
	}

	if !locked {
		err = hostCreateInProgressErr
		return err
	}

	queries := []string{
		"DELETE FROM tokens WHERE user_id = $1",
		"DELETE FROM container_collabs WHERE user_id = $1",
		"UPDATE free_queue SET deleted_at = now() WHERE user_id = $1 AND deleted_at IS NULL",
		"DELETE FROM verifications WHERE user_id = $1;",
		"UPDATE users SET deleted_at = now() WHERE id = $1",
	}

	for _, query := range queries {
		_, err = tx.Exec(query, env.User.ID)
		if err != nil {
			return err
		}
	}

	bp, err := billingprofile.Find(env.DB, env.User.ID)
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
		WHERE user_id = $2`, billingprofile.StateNoCard, env.User.ID)
	if err != nil {
		return err
	}

	if bp.StripeID.Valid {
		params := &stripe.CardListParams{Customer: bp.StripeID.String}
		i := card.List(params)
		for !i.Stop() {
			var c *stripe.Card
			c, err = i.Next()
			if err != nil {
				return err
			}

			err = card.Del(c.ID, &stripe.CardParams{Customer: bp.StripeID.String})
			if err != nil {
				return err
			}
		}
	}

	env.Tracker.SetProfileProps("set", env.User.ID, map[string]interface{}{
		"deleted_date": time.Now(),
	}, env.Logger)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func canDeleteUser(d db.QueryReceiver, u *user.User) error {
	var hExists bool
	err := d.Get(&hExists, `SELECT
		COUNT(*) >= 1
		FROM hosts
		WHERE user_id = $1 AND terminated_at IS NULL AND state != 'orphan' AND preprovisioned IS FALSE LIMIT 1;`, u.ID)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if hExists {
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
	res, err := biller.InvoiceItemsForUser(d, nil, u.ID, start, end)
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

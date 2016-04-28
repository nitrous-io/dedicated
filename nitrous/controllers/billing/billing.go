package billing

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/jinzhu/now"
	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/controllers/shared"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/invoicemanualattempt"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/stripememo"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/permission"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	stripe "github.com/stripe/stripe-go"
	strinv "github.com/stripe/stripe-go/invoice"
)

var (
	errInvoiceNotFound   = httperror.New(http.StatusNotFound, "invoice_not_found", "invoice does not exist")
	errInvoiceNotPayable = httperror.New(http.StatusBadRequest, "invoice_not_payable", "invoice not payable")
	errInvoicePaid       = httperror.New(http.StatusBadRequest, "invoice_paid", "invoice has already been paid")

	errInvoiceUserPaying = httperror.New(http.StatusBadRequest, "invoice_user_paying", "a user can only pay one invoice at a time")

	errCardDeclined = httperror.New(http.StatusBadRequest, "card_declined", "Your credit card was declined. Try contacting the bank directly for assistance.")
	errCardExpired  = httperror.New(http.StatusBadRequest, "card_expired", "Your credit card has expired. Please try again with a recent card.")

	errPayInvoiceForbidden = httperror.New(http.StatusForbidden, "pay_invoice_forbidden", "You do not have permission to pay invoices")
)

func CurrentUsage(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	start := now.BeginningOfMonth()
	end := time.Now()

	availableCredits, err := credit.AvailableCredits(env.DB, env.User.ID)
	if err != nil {
		logrus.Errorf("CurrentUsage: error fetching credit.AvailableCredits. Err: %+v", err)
		return err
	}
	availableCreditsDollars := float64(availableCredits) / 100.0

	pHMap, err := plan.PlanPricingHostMap(env.DB, env.User.ID)
	if err != nil {
		logrus.Errorf("CurrentUsage: error fetching plan.PlanPricingHostMap. Err: %+v", err)
		return err
	}

	res, err := biller.InvoiceItemsForUser(env.DB, env.User.ID, start, end)
	if err != nil {
		logrus.Errorf("CurrentUsage: error fetching biller.InvoiceItemsForUser. Err: %+v", err)
		return err
	}

	if len(res.Items) == 0 {
		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"items":                     []interface{}{},
			"total_charge":              0,
			"available_credits_dollars": availableCreditsDollars,
		}
		return nil
	}

	hMap, err := fetchHosts(env.DB, env.User)
	if err != nil {
		logrus.Errorf("CurrentUsage: error fetching fetchHosts. Err: %+v", err)
		return err
	}

	hostIDs := make([]int64, len(hMap))
	for hID := range hMap {
		hostIDs = append(hostIDs, hID)
	}

	usages, err := hosttransition.UsageForHosts(env.DB, hostIDs, start, end)
	if err != nil {
		logrus.Errorf("billing: Unable to fetch transition breakdown for user %d, err: %v", env.User.ID, err)
	}

	sItems := []serializers.InvoiceItem{}
	for _, inv := range res.Items {
		ser := serializers.SerializeInvoiceItem(&inv)
		if hst, ok := hMap[ser.HostID]; ok {
			ser.HostSlug = hst.Slug
			ser.Description = invoiceitem.ItemDescription(&inv, hMap[ser.HostID])
		} else {
			ser.Description = "Information not available at this time"
			logrus.Errorf("billing: Unable to locate host %d in hMap for user %d", ser.HostID, env.User.ID)
		}
		if price, ok := pHMap[ser.HostID]; ok {
			ser.TotalCharge = inv.TotalCharge(&plan.Plan{Price: price.Price, BasePrice: price.BasePrice})
		} else {
			ser.TotalCharge = inv.UncappedTotalCharge()
			logrus.Errorf("billing: Unable to locate host %d in pHMap for user %d", ser.HostID, env.User.ID)
		}

		if hUsages, ok := usages[ser.HostID]; ok {
			ser.Usage = serializers.SerializeHostUsages(hUsages)
		}
		sItems = append(sItems, *ser)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"items":                     sItems,
		"total_charge":              res.TotalCharge,
		"available_credits_dollars": availableCreditsDollars,
	}

	return nil
}

// no route to this at present -- use param as feature flag to test before going live
func StripeInvoices(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	bp, err := shared.BillingProfileForRequest(env, r)
	if err != nil {
		return err
	}
	if !bp.StripeID.Valid {
		// user has no stripe history = no invoices
		// otherwise the call to Stripe API will fail with
		// `invalid_request_error` due to missing 'customer' field
		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"invoices": []interface{}{},
		}
		return nil
	}
	invoices, err := stripememo.Invoices(env.DB, bp)
	if err != nil {
		return err
	}
	serInvs := []*stripememo.InvoiceSerialized{}
	for _, inv := range invoices {
		si, err := inv.StripeInvoice()
		if err != nil {
			logrus.Warnf("billing.StripeInvoices: error with invoice bpid %d (invoiced at %v): %s", inv.BillingProfileID, inv.InvoicedAt, err)
			continue
		}
		ni := stripememo.NitrousInvoice{*si}
		if !ni.ShowInHistory() {
			continue
		}
		sInv := serializers.SerializeStripeInvoice(inv)
		sInv.UserID = env.User.ID
		serInvs = append(serInvs, sInv)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"invoices": serInvs,
	}
	return nil
}

func Invoices(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if r.FormValue("from_stripe") == "1" {
		return StripeInvoices(env, w, r)
	}
	bp, err := shared.BillingProfileForRequest(env, r)
	if err != nil {
		return err
	}
	invoices, err := fetchRecentInvoices(env.DB, bp.ID)
	if err != nil {
		return err
	}

	if len(invoices) == 0 {
		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"invoices": []interface{}{},
		}
		return nil
	}

	invIDs := []string{}
	serInvs := []*serializers.Invoice{}
	for _, inv := range invoices {
		invIDs = append(invIDs, strconv.FormatInt(inv.ID, 10))
		sInv := serializers.SerializeInvoice(&inv)
		serInvs = append(serInvs, sInv)
	}

	invItems, err := fetchInvoiceItems(env.DB, invIDs)
	if err != nil {
		return err
	}

	pHMap, err := plan.PlanPricingHostMapWithSubscription(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	hMap, err := fetchHosts(env.DB, env.User)
	if err != nil {
		return err
	}

	for _, inv := range serInvs {
		hostIDs := []int64{}
		for _, item := range invItems {
			if item.InvoiceID == inv.ID && item.HostID.Valid {
				hostIDs = append(hostIDs, item.HostID.Int64)
			}
		}
		usages, err := hosttransition.UsageForHosts(env.DB, hostIDs, *inv.PeriodStart, *inv.PeriodEnd)
		if err != nil {
			logrus.Errorf("billing: Unable to fetch transition breakdown for user %d, err: %v", env.User.ID, err)
		}
		for _, item := range invItems {
			if item.InvoiceID == inv.ID {
				if item.Credit.Valid {
					inv.TotalCredit += item.Credit.Int64
				} else {
					ser := serializers.SerializeInvoiceItem(&item)
					if item.ItemType == invoiceitem.ItemTypeSubscription {
						ser.Description = invoiceitem.ItemDescription(&item, nil)
						ser.TotalCharge = item.TotalStripeCharge.Int64
						inv.Items = append(inv.Items, *ser)
						continue
					}

					if hst, ok := hMap[ser.HostID]; ok {
						ser.HostSlug = hst.Slug
						ser.Description = invoiceitem.ItemDescription(&item, hMap[ser.HostID])
					} else {
						ser.Description = "Information not available at this time"
						logrus.Errorf("billing: Unable to locate host %d in hMap for user %d", ser.HostID, env.User.ID)
					}
					if price, ok := pHMap[ser.HostID]; ok {
						ser.TotalCharge = item.TotalCharge(&plan.Plan{Price: price.Price, BasePrice: price.BasePrice})
					} else {
						ser.TotalCharge = item.UncappedTotalCharge()
						logrus.Errorf("billing: Unable to locate host %d in pHMap for user %d", ser.HostID, env.User.ID)
					}
					hUsages, ok := usages[ser.HostID]
					if ok {
						ser.Usage = serializers.SerializeHostUsages(hUsages)
					}

					inv.Items = append(inv.Items, *ser)
				}
			}
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"invoices": serInvs,
	}
	return nil
}

func PayInvoice(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if !permission.PayInvoice(env.Token, env.User) {
		return errPayInvoiceForbidden
	}

	// we use a transaction purely to acquire a lock on the same user paying other invoices
	// it is rolled back whenever this func finishes up
	// IMPORTANT: this implies the transaction should *not* be used in
	// functions that modify db!
	txLock, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		txLock.Rollback()
	}()
	locked, err := db.TransactionLock(txLock, db.INVOICE_USER_PAY_LOCK, int(env.User.ID))
	if err != nil {
		return err
	}
	if !locked {
		return errInvoiceUserPaying
	}

	inv, err := invoiceFromParam(env.DB, mux.Vars(r)["id"])
	if err != nil {
		if err == sql.ErrNoRows {
			return errInvoiceNotFound
		}
		return err
	}
	if !inv.BillingProfileID.Valid {
		return errInvoiceNotFound
	}

	bp, err := billingprofile.FindByID(env.DB, inv.BillingProfileID.Int64)
	if err != nil {
		return err
	}
	if bp.UserID != env.User.ID {
		return errInvoiceNotFound
	}

	if !inv.StripeID.Valid || inv.State == invoice.StateDeferred {
		return errInvoiceNotPayable
	}

	if inv.State == invoice.StatePaid {
		return errInvoicePaid
	}

	i, err := strinv.Pay(inv.StripeID.String, nil)
	if err != nil {
		if err := invoicemanualattempt.CreateForInvoice(env.DB, inv.ID, false, err.Error()); err != nil {
			logrus.Errorf("Failure recording manual attempt for invoice %d for bp %d, err: %v", inv.ID, bp.ID, err)
		}
		logrus.Errorf("Error manual paying Stripe invoice %d for bp %d, err: %v", inv.ID, bp.ID, err)
		sErr, ok := err.(*stripe.Error)
		if !ok {
			return sErr
		}

		if sErr.Code == stripe.ExpiredCard {
			return errCardExpired
		} else if sErr.Code == stripe.CardDeclined {
			return errCardDeclined
		}
		return err
	}

	if !i.Paid {
		return fmt.Errorf("Unexpected Stripe invoice %d for bp %d not paid: %v", inv.ID, bp.ID, i)
	}

	if err := invoicemanualattempt.CreateForInvoice(env.DB, inv.ID, true, ""); err != nil {
		logrus.Errorf("Failure recording manual attempt for invoice %d for bp %d, err: %v", inv.ID, bp.ID, err)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"paid": i.Paid,
	}
	return nil
}

func invoiceFromParam(db db.QueryReceiver, idParam string) (*invoice.Invoice, error) {
	// we support both a stripe id and an integer id (db)
	if strings.HasPrefix(idParam, "in_") { // stripe id
		return invoice.FindByStripeID(db, idParam)
	} else {
		invID, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil {
			return nil, err
		}
		return invoice.Find(db, invID)
	}
}

func fetchHosts(db *db.Database, user *user.User) (map[int64]*host.Host, error) {
	hosts, err := host.FindAllWithTerminated(db, user.ID)
	if err != nil {
		return nil, err
	}
	hMap := make(map[int64]*host.Host)
	for _, host := range hosts {
		hMap[host.ID] = host
	}
	return hMap, nil
}

func fetchRecentInvoices(d db.QueryReceiver, bpID int64) ([]invoice.Invoice, error) {
	invoices := []invoice.Invoice{}
	err := d.Select(&invoices, "SELECT * FROM invoices WHERE billing_profile_id = $1 ORDER BY id DESC LIMIT 6;", bpID)
	if err != nil {
		return nil, err
	}
	return invoices, nil
}

func fetchInvoiceItems(d db.QueryReceiver, invoiceIDs []string) ([]invoiceitem.InvoiceItem, error) {
	items := []invoiceitem.InvoiceItem{}
	sql := fmt.Sprintf("SELECT * FROM invoice_items WHERE invoice_id IN (%s) ORDER BY id DESC;", strings.Join(invoiceIDs, ","))
	err := d.Select(&items, sql)
	if err != nil {
		return nil, err
	}
	return items, nil
}

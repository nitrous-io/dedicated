package billing

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/jinzhu/now"
	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/invoicemanualattempt"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/stripe/stripe-go"
	strinv "github.com/stripe/stripe-go/invoice"
)

var (
	errInvoiceNotFound   = httperror.New(http.StatusNotFound, "invoice_not_found", "invoice does not exist")
	errInvoiceNotPayable = httperror.New(http.StatusBadRequest, "invoice_not_payable", "invoice not payable")
	errInvoicePaid       = httperror.New(http.StatusBadRequest, "invoice_paid", "invoice has already been paid")

	errInvoiceUserPaying = httperror.New(http.StatusBadRequest, "invoice_user_paying", "a user can only pay one invoice at a time")

	errCardDeclined = httperror.New(http.StatusBadRequest, "card_declined", "Your Credit Card was declined. Try contacting the bank directly for assistance.")
	errCardExpired  = httperror.New(http.StatusBadRequest, "card_expired", "Your Credit Card has expired. Please try again with a recent card.")
)

func CurrentUsage(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	start := now.BeginningOfMonth()
	end := time.Now()

	availableCredits, err := credit.AvailableCredits(env.DB, env.User.ID)
	if err != nil {
		return err
	}
	availableCreditsDollars := float64(availableCredits) / 100.0

	pHMap, err := plan.PlanPricingHostMap(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	res, err := biller.InvoiceItemsForUser(env.DB, nil, env.User.ID, start, end)
	if err != nil {
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
		return err
	}

	hostIDs := make([]int64, len(hMap))
	for hID := range hMap {
		hostIDs = append(hostIDs, hID)
	}

	usages, err := hosttransition.UsageForHosts(env.DB, hostIDs, start, end)
	if err != nil {
		env.Logger.Errorf("billing: Unable to fetch transition breakdown for user %d, err: %v", env.User.ID, err)
	}

	sItems := []serializers.InvoiceItem{}
	for _, inv := range res.Items {
		ser := serializers.SerializeInvoiceItem(&inv)
		if hst, ok := hMap[ser.HostID]; ok {
			ser.HostSlug = hst.Slug
			ser.Description = invoiceitem.ItemDescription(&inv, hMap[ser.HostID])
		} else {
			ser.Description = "Information not available at this time"
			env.Logger.Errorf("billing: Unable to locate host %d in hMap for user %d", ser.HostID, env.User.ID)
		}
		if price, ok := pHMap[ser.HostID]; ok {
			ser.TotalCharge = inv.TotalCharge(&plan.Plan{Price: price.Price, BasePrice: price.BasePrice})
		} else {
			ser.TotalCharge = inv.UncappedTotalCharge()
			env.Logger.Errorf("billing: Unable to locate host %d in pHMap for user %d", ser.HostID, env.User.ID)
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

func Invoices(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	invoices, err := fetchRecentInvoices(env.DB, env.User.ID)
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

	pHMap, err := plan.PlanPricingHostMap(env.DB, env.User.ID)
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
			env.Logger.Errorf("billing: Unable to fetch transition breakdown for user %d, err: %v", env.User.ID, err)
		}
		for _, item := range invItems {
			if item.InvoiceID == inv.ID {
				if item.Credit.Valid {
					inv.TotalCredit += item.Credit.Int64
				} else {
					ser := serializers.SerializeInvoiceItem(&item)
					if hst, ok := hMap[ser.HostID]; ok {
						ser.HostSlug = hst.Slug
						ser.Description = invoiceitem.ItemDescription(&item, hMap[ser.HostID])
					} else {
						ser.Description = "Information not available at this time"
						env.Logger.Errorf("billing: Unable to locate host %d in hMap for user %d", ser.HostID, env.User.ID)
					}
					if price, ok := pHMap[ser.HostID]; ok {
						ser.TotalCharge = item.TotalCharge(&plan.Plan{Price: price.Price, BasePrice: price.BasePrice})
					} else {
						ser.TotalCharge = item.UncappedTotalCharge()
						env.Logger.Errorf("billing: Unable to locate host %d in pHMap for user %d", ser.HostID, env.User.ID)
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
	// we use a transaction purely to acquire a lock on the same user paying other invoices
	// it is rolled back whenever this func finishes up
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

	invIDStr := mux.Vars(r)["id"]
	invID, err := strconv.ParseInt(invIDStr, 10, 64)
	if err != nil {
		return err
	}

	inv := &invoice.Invoice{}
	if err := env.DB.Get(inv, `SELECT * FROM invoices WHERE id = $1 AND user_id = $2`, invID, env.User.ID); err != nil {
		if err == sql.ErrNoRows {
			return errInvoiceNotFound
		}
		return err
	}

	if !inv.StripeID.Valid || inv.State == invoice.StateDeferred {
		return errInvoiceNotPayable
	}

	if inv.State == invoice.StatePaid {
		return errInvoicePaid
	}

	i, err := strinv.Pay(inv.StripeID.String, nil)
	if err != nil {
		if err := invoicemanualattempt.CreateForInvoice(env.DB, invID, false, err.Error()); err != nil {
			env.Logger.Errorf("Failure recording manual attempt for invoice %d for user %d, err: %v", invID, env.User.ID, err)
		}
		env.Logger.Errorf("Error manual paying Stripe invoice %d for user %d, err: %v", invID, env.User.ID, err)
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
		return fmt.Errorf("Unexpected Stripe invoice %d for user %d not paid: %v", invID, env.User.ID, i)
	}

	if err := invoicemanualattempt.CreateForInvoice(env.DB, invID, true, ""); err != nil {
		env.Logger.Errorf("Failure recording manual attempt for invoice %d for user %d, err: %v", invID, env.User.ID, err)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"paid": i.Paid,
	}
	return nil
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

func fetchRecentInvoices(d db.QueryReceiver, userID int64) ([]invoice.Invoice, error) {
	invoices := []invoice.Invoice{}
	err := d.Select(&invoices, "SELECT * FROM invoices WHERE user_id = $1 ORDER BY id DESC LIMIT 6;", userID)
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

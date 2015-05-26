package biller

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/now"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/stripe/stripe-go"
	strinv "github.com/stripe/stripe-go/invoice"
	striitem "github.com/stripe/stripe-go/invoiceitem"
)

const (
	// the maximum ratio we permit the plan price to be greater than the host total charge
	MaxPriceDiffRatio = 1.5
	// the maximum ratio we permit the host total charge to be greater than the plan price
	MaxHostPriceDiffRatio = 1.01
	// the minimum value (in cents) a invoice must equal to send off to Stripe
	// otherwise it will rollover to the next invoice
	MinimumInvoiceAmount = 100
)

var (
	ErrNoUsage            = errors.New("biller: no usage from user")
	ErrInvoiceUserRunning = errors.New("biller: invoice createOrUpdate() already running for user")
)

type Biller struct {
	db     *db.Database
	logger *logger.Logger
	user   *user.User

	periodStart *time.Time
	periodEnd   *time.Time
}

type InvoiceResult struct {
	Invoice *invoice.Invoice
	Items   []invoiceitem.InvoiceItem
}

type updatedAvailableCredits struct {
	Consumed  int64
	Available int64
}

func New(d *db.Database, l *logger.Logger, u *user.User, periodStart, periodEnd *time.Time) (*Biller, error) {
	if d == nil {
		return nil, errors.New("Database must be provided")
	}
	if l == nil {
		return nil, errors.New("Logger must be provided")
	}
	if u == nil {
		return nil, errors.New("User must be provided")
	}
	return &Biller{
		db:          d,
		logger:      l,
		user:        u,
		periodStart: periodStart,
		periodEnd:   periodEnd,
	}, nil
}

func (b *Biller) Process() error {
	i, err := b.createOrUpdateInvoice()
	if err != nil {
		if err == ErrNoUsage {
			if err := b.submitRecentlyDeferred(); err != nil {
				return err
			}
			b.logger.Infof("biller: no usage for user %d", b.user.ID)
			return nil
		}
		return err
	}

	return b.processStripe(i)
}

func (b *Biller) createOrUpdateInvoice() (res *InvoiceResult, err error) {
	tx, err := b.db.Beginx()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err == nil || res != nil {
			return
		}

		tx.Rollback()
	}()

	err = b.acquireLock(tx)
	if err != nil {
		return nil, err
	}

	itemResult, err := InvoiceItemsForUser(tx, b.logger, b.user.ID, *b.periodStart, *b.periodEnd)
	if err != nil {
		return nil, fmt.Errorf("biller: failed retrieving invoice items for user %d, err: %v", b.user.ID, err)
	}
	if len(itemResult.Items) == 0 {
		return nil, ErrNoUsage
	}

	if err = b.sanityCheckInvoiceItems(itemResult.Items); err != nil {
		return nil, err
	}

	var isNew bool
	i, err := invoice.ForPeriod(tx, b.user.ID, *b.periodStart, *b.periodEnd)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, fmt.Errorf("biller: encountered error when try to find existing invoice for user %d, err: %v", b.user.ID, err)
		}
		isNew = true
		i = &invoice.Invoice{
			UserID:      b.user.ID,
			PeriodStart: b.periodStart,
			PeriodEnd:   b.periodEnd,
			State:       invoice.StateUnpaid,
			Attempts:    0,
		}
	} else if i.State == invoice.StatePaid {
		return nil, fmt.Errorf("biller: cannot touch invoice %d, already paid", i.ID)
	}
	i.TotalCharge = (itemResult.TotalCharge - i.CreditAmount) // reconcile with existing credited amount
	if i.TotalCharge == 0 {
		i.State = invoice.StateNotPayable
	} else {
		i.State = invoice.StateUnpaid
	}

	if isNew {
		err = i.Insert(tx)
	} else {
		err = i.Update(tx)
	}
	if err != nil {
		return nil, fmt.Errorf("biller: failure inserting/updating invoice %d for user %d, err: %v", i.ID, i.UserID, err)
	} else {
		b.logger.Infof("biller: created/updated invoice %d for user %d, total charge = %d", i.ID, i.UserID, i.TotalCharge)
	}

	cr, err := updateAvailableCredits(tx, i.UserID, i.ID, i.TotalCharge)
	if err != nil {
		return nil, fmt.Errorf("biller: failure acquiring credits for invoice %d for user %d, err: %v", i.ID, i.UserID, err)
	}

	if cr != nil {
		// if this invoice is being processed again (due to an update) need to consolidate previous total with the current credits
		i.AvailableCredit = sql.NullInt64{cr.Available + i.CreditAmount, true}
		i.TotalCharge += cr.Consumed
		i.CreditAmount -= cr.Consumed
		if i.TotalCharge == 0 {
			i.State = invoice.StateNotPayable
		}
		if err = i.Update(tx); err != nil {
			return nil, fmt.Errorf("biller: failure deducting %d credits (of %d) from invoice %d for user %d, err: %v", cr.Consumed, cr.Available, i.ID, i.UserID, err)
		}
	}
	res = &InvoiceResult{Invoice: i, Items: itemResult.Items}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("biller: failure committing invoice %d for user %d, err: %v", i.ID, i.UserID, err)
	}

	res.Items, err = b.applyInvoiceItems(i, isNew, itemResult.Items)
	if err != nil {
		return nil, fmt.Errorf("biller: failure inserting/updating invoice %d items for %d, err: %v", i.ID, i.UserID, err)
	}

	return res, nil
}

func (b *Biller) submitRecentlyDeferred() error {
	b.logger.Debugf("biller: looking for deferred invoices to submit for user %d", b.user.ID)

	inv := &invoice.Invoice{}
	err := b.db.Get(inv, `SELECT * FROM invoices WHERE user_id = $1 AND state = $2 ORDER BY id DESC LIMIT 1`,
		b.user.ID, invoice.StateDeferred)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	items, err := invoiceitem.FindAllForInvoice(b.db, inv.ID)
	if err != nil {
		return err
	}

	res := &InvoiceResult{
		Invoice: inv,
		Items:   items,
	}
	return b.processStripe(res)
}

func (b *Biller) processStripe(i *InvoiceResult) error {
	inv := i.Invoice
	items := i.Items

	if b.user.DeletedAt.Valid && !b.user.BannedAt.Valid {
		return fmt.Errorf("biller: skipping stripe, user %d has been DELETED", b.user.ID)
	}

	bp, err := billingprofile.Find(b.db, b.user.ID)
	if err != nil {
		return fmt.Errorf("biller: skipping stripe, failure obtaining billing profile for user %d, err: %v", b.user.ID, err)
	}
	if bp.State == billingprofile.StateNoCard {
		return fmt.Errorf("biller: skipping stripe, billing profile for user %d is in state no_card", b.user.ID)
	}
	if bp.State == billingprofile.StateExempt {
		inv.State = invoice.StateNotPayable
		if err := inv.Update(b.db); err != nil {
			b.logger.Errorf("biller: failure updating invoice %d as StateNotPayable, err: %v", inv.ID, err)
		}
		return fmt.Errorf("biller: skipping stripe, billing profile for user %d is in state exempt", b.user.ID)
	}
	if bp.State == billingprofile.StateFree {
		inv.State = invoice.StateNotPayable
		if err := inv.Update(b.db); err != nil {
			b.logger.Errorf("biller: failure updating invoice %d as StateNotPayable, err: %v", inv.ID, err)
		}
		return fmt.Errorf("biller: skipping stripe, billing profile for user %d is in state free", b.user.ID)
	}
	if bp.StripeID.String == "" {
		return fmt.Errorf("biller: skipping stripe, billing profile for user %d lacks StripeID", b.user.ID)
	}

	pHMap, err := b.planHostMap()
	if err != nil {
		return fmt.Errorf("biller: failed retrieving mapping of hosts to plan pricing for user %d, err: %v", b.user.ID, err)
	}

	if inv.TotalCharge == 0 {
		b.logger.Infof("biller: charge 0, not contacting stripe to charge for items in invoice %d", inv.ID)
	} else {
		b.logger.Infof("biller: contacting stripe for invoice %d", inv.ID)
		if err := b.billStripeInvoiceItems(bp, pHMap, inv, items); err != nil {
			return fmt.Errorf("biller: failure billing stripe for invoice %d, err: %v", inv.ID, err)
		}
	}

	sendInvoice := inv.TotalCharge >= MinimumInvoiceAmount

	var upI *stripe.Invoice
	if !sendInvoice {
		// query stripe for the upcoming invoice preview (invoice has not been committed)
		upI, err = b.upcomingInvoicePreview(bp)
		if err != nil {
			return fmt.Errorf("biller: failure obtaining upcoming invoice total for user %d, err: %v", b.user.ID, err)
		}
		if upI == nil {
			sendInvoice = false
		} else {
			sendInvoice = upI.Amount != 0
		}
	}

	if sendInvoice {
		upI, err = b.billStripeInvoice(bp, inv)
		if err != nil {
			return fmt.Errorf("biller: failure creating invoice for %d, err: %v", inv.ID, err)
		}
		if upI != nil {
			// associate other invoices with this invoice
			return b.associateDeferredInvoices(upI, inv)
		}
	} else {
		if inv.TotalCharge != 0 {
			inv.State = invoice.StateDeferred
			if err := inv.Update(b.db); err != nil {
				return fmt.Errorf("biller: failure updating invoice %d as deferred, err: %v", inv.ID, err)
			}
		}
	}
	return nil
}

func (b *Biller) upcomingInvoicePreview(bp *billingprofile.BillingProfile) (*stripe.Invoice, error) {
	return strinv.GetNext(&stripe.InvoiceParams{Customer: bp.StripeID.String})
}

func (b *Biller) billStripeInvoice(bp *billingprofile.BillingProfile, i *invoice.Invoice) (*stripe.Invoice, error) {
	if i.StripeID.String == "" {
		inv, err := strinv.New(&stripe.InvoiceParams{Customer: bp.StripeID.String})
		if err != nil {
			return nil, err
		}
		i.StripeID = sql.NullString{inv.ID, true}
		return inv, i.Update(b.db)
	} else {
		return strinv.Get(i.StripeID.String, nil)
	}
}

func (b *Biller) billStripeInvoiceItems(bp *billingprofile.BillingProfile, planHostMap map[int64]plan.PlanPricing, i *invoice.Invoice, items []invoiceitem.InvoiceItem) error {
	hosts, err := host.FindAllWithTerminated(b.db, b.user.ID)
	if err != nil {
		return err
	}
	tx, err := b.db.Beginx()
	if err != nil {
		return err
	}
	for _, invI := range items {
		var matchingHost *host.Host
		for _, hst := range hosts {
			if hst.ID == invI.HostID.Int64 {
				matchingHost = hst
				break
			}
		}

		// if this invoice item references a missing host and it is not a credit, raise an error
		if matchingHost == nil && !invI.Credit.Valid {
			tx.Rollback()
			return fmt.Errorf("Could not locate host %d for invoice %d", invI.HostID.Int64, invI.ID)
		}
		var pl *plan.Plan
		if !invI.Credit.Valid {
			planPrice, ok := planHostMap[invI.HostID.Int64]
			if !ok {
				return fmt.Errorf("Could not locate plan pricing for host %d for invoice %d", invI.HostID.Int64, invI.ID)
			}
			pl = &plan.Plan{Price: planPrice.Price, BasePrice: planPrice.BasePrice}
		}

		desc := invoiceitem.ItemDescription(&invI, matchingHost)
		if invI.StripeID.String == "" {
			ii, err := striitem.New(&stripe.InvoiceItemParams{
				Customer: bp.StripeID.String,
				Amount:   invI.TotalCharge(pl),
				Currency: "usd",
				Desc:     desc,
			})
			if err != nil {
				tx.Rollback()
				return err
			}
			invI.StripeID = sql.NullString{ii.ID, true}
			err = invI.Update(tx)
			if err != nil {
				tx.Rollback()
				return err
			}
		} else {
			_, err := striitem.Update(
				invI.StripeID.String,
				&stripe.InvoiceItemParams{
					Amount: invI.TotalCharge(pl),
					Desc:   desc,
				})
			if err != nil {
				tx.Rollback()
				return err
			}
		}
		b.logger.Infof("biller: submitted invoice item %d, Total Charge: %d", invI.ID, invI.TotalCharge(pl))
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

func (b *Biller) applyInvoiceItems(i *invoice.Invoice, isNew bool, items []invoiceitem.InvoiceItem) ([]invoiceitem.InvoiceItem, error) {
	existing := []invoiceitem.InvoiceItem{}
	var err error
	if !isNew {
		existing, err = invoiceitem.FindAllForInvoice(b.db, i.ID)
		if err != nil {
			return nil, err
		}
		isNew = len(existing) == 0
	}

	tx, err := b.db.Beginx()
	if err != nil {
		return nil, err
	}
	for indx, inv := range items {
		// perform an update if have existing items
		if !isNew {
			for _, ex := range existing {
				if inv.HostID == ex.HostID && inv.ItemType == ex.ItemType {
					inv.ID = ex.ID
					inv.StripeID = ex.StripeID
				}
			}
		}

		inv.InvoiceID = i.ID
		op := inv.Insert
		if inv.ID > 0 {
			op = inv.Update
		}
		err = op(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		items[indx] = inv
	}

	// apply any credits that may be needed
	totalAmountCredited := int64(0)
	for _, itm := range existing {
		if itm.Credit.Valid {
			totalAmountCredited += itm.Credit.Int64
		}
	}
	if totalAmountCredited != i.CreditAmount {
		// need to create invoice item to cover difference
		itm := &invoiceitem.InvoiceItem{
			InvoiceID: i.ID,
			Credit:    sql.NullInt64{i.CreditAmount - totalAmountCredited, true},
			ItemType:  invoiceitem.ItemTypeCredit,
		}
		if err := itm.Insert(tx); err != nil {
			tx.Rollback()
			return nil, err
		}
		items = append(items, *itm)
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}
	return items, err
}

func (b *Biller) associateDeferredInvoices(upI *stripe.Invoice, i *invoice.Invoice) error {
	if upI.Lines == nil {
		return nil
	}

	if len(upI.Lines.Values) == 0 {
		return nil
	}

	var itemIDs []string
	for _, item := range upI.Lines.Values {
		itemIDs = append(itemIDs, fmt.Sprintf("invoice_items.stripe_id = '%s'", item.ID))
	}

	stmt := fmt.Sprintf("SELECT DISTINCT ON (invoices.id) invoices.* FROM invoices INNER JOIN invoice_items ON (invoices.id = invoice_items.invoice_id) WHERE invoices.id != $1 AND (%s);", strings.Join(itemIDs, " OR "))
	otherInvs := []invoice.Invoice{}
	err := b.db.Select(&otherInvs, stmt, i.ID)
	if err != nil {
		return err
	}
	if len(otherInvs) == 0 {
		return nil
	}

	tx, err := b.db.Beginx()
	if err != nil {
		return err
	}

	for _, dInv := range otherInvs {
		if dInv.CoveredBy.Valid && dInv.CoveredBy.Int64 != 0 {
			b.logger.Warnf("biller: Invoice %d cannot be associated with %d, as it is already associated with %d", dInv.ID, i.ID, dInv.CoveredBy.Int64)
			continue
		}
		dInv.CoveredBy = sql.NullInt64{i.ID, true}
		if err := dInv.Update(tx); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (b *Biller) sanityCheckInvoiceItems(items []invoiceitem.InvoiceItem) error {
	mapping, err := b.planHostMap()
	if err != nil {
		return err
	}
	for _, item := range items {
		planPrice, ok := mapping[item.HostID.Int64]
		if !ok {
			return fmt.Errorf("biller: Could not locate plan for user %d host %d", b.user.ID, item.HostID.Int64)
		}

		if item.ItemType == invoiceitem.ItemTypeFreeCompute {
			b.logger.Infof("InvoiceItem for host %d is free, no checking required", item.HostID)
			continue
		}

		pl := &plan.Plan{Price: planPrice.Price, BasePrice: planPrice.BasePrice}
		targetPrice := pl.Price
		if item.ItemType == invoiceitem.ItemTypeBaseEBS {
			targetPrice = pl.BasePrice
		}
		totalCh := float64(item.TotalCharge(pl))
		b.logger.Infof("biller: InvoiceItem UserID: %d HostID: %d Hours: %d HourlyPrice: %d TotalCharge: %.2f Monthly Plan: %d", b.user.ID, item.HostID.Int64, item.Hours.Int64, item.HourlyPrice.Int64, totalCh, targetPrice)

		uncappedRatio := float64(item.UncappedTotalCharge()) / float64(targetPrice)
		if uncappedRatio > MaxHostPriceDiffRatio {
			return fmt.Errorf("biller: Pricing %d for user %d host %d exceeds plan price ratio %d by %f", item.UncappedTotalCharge(), b.user.ID, item.HostID.Int64, targetPrice, uncappedRatio)
		}
		if item.Hours.Int64 == invoiceitem.MaxHours {
			priceRatio := float64(targetPrice) / totalCh
			if priceRatio >= MaxPriceDiffRatio {
				return fmt.Errorf("biller: Pricing %d for user %d host %d exceeds ratio %f with %f", item.TotalCharge(pl), b.user.ID, item.HostID.Int64, MaxPriceDiffRatio, priceRatio)
			}
		}
	}
	return nil
}

func (b *Biller) acquireLock(tx *sqlx.Tx) error {
	locked, err := db.TransactionLock(tx, db.INVOICE_USER_LOCK, int(b.user.ID))
	if err != nil {
		return err
	}
	if !locked {
		return ErrInvoiceUserRunning
	}
	return nil
}

func updateAvailableCredits(tx *sqlx.Tx, userID int64, invID int64, amount int64) (*updatedAvailableCredits, error) {
	// insert into credits ONLY if the number of available of credits is greater than 0
	// using FOR UPDATE to ensure the rows are locked from other queries modifying
	availableCredits, err := credit.AvailableCredits(tx, userID)
	if err != nil {
		return nil, err
	}
	if availableCredits == 0 {
		return nil, nil
	}
	if availableCredits < amount {
		amount = availableCredits
	}
	deduction := amount * -1
	_, err = tx.Exec(`INSERT INTO credits(user_id, invoice_id, amount) VALUES ($1, $2, $3)`, userID, invID, deduction)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &updatedAvailableCredits{
		Consumed:  deduction,
		Available: availableCredits,
	}, nil
}

func (b *Biller) planHostMap() (map[int64]plan.PlanPricing, error) {
	return plan.PlanPricingHostMap(b.db, b.user.ID)
}

func ProcessForAllUsers(d *db.Database, l *logger.Logger, periodStart, periodEnd *time.Time) {
	pS, pE := StartEndTimeOrDefault(periodStart, periodEnd)

	l.Debugf("biller: Processing period %v to %v", pS, pE)

	lastId := int64(-1)
	usersExhausted := false

	// page through the hosts in the DB, 100 at a time
	for !usersExhausted {
		users := []user.User{}
		err := d.Select(&users, `SELECT * FROM users WHERE service_account IS FALSE AND id > $1 ORDER BY id ASC LIMIT 100;`, lastId)
		if err != nil && err != sql.ErrNoRows {
			l.FatalExit(err)
		}

		if err == sql.ErrNoRows {
			usersExhausted = true
			break
		}

		if len(users) == 0 {
			usersExhausted = true
			break
		}

		for _, user := range users {
			if user.ID > lastId {
				lastId = user.ID
			}
			inst, err := New(d, l, &user, &pS, &pE)
			if err != nil {
				l.FatalExit(err)
			}
			err = inst.Process()
			if err != nil {
				l.Error(err)
			}
		}
	}
}

func StartEndTimeOrDefault(periodStart, periodEnd *time.Time) (time.Time, time.Time) {
	var pS time.Time
	var pE time.Time

	if periodStart == nil {
		base := now.BeginningOfMonth().Add(-1 * time.Nanosecond)
		pS = now.New(base).BeginningOfMonth()
	} else {
		pS = *periodStart
	}

	if periodEnd == nil {
		pE = now.New(pS).EndOfMonth()
	} else {
		pE = *periodEnd
	}

	return pS, pE
}

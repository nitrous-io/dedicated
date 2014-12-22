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
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/stripe/stripe-go"
	strinv "github.com/stripe/stripe-go/invoice"
	striitem "github.com/stripe/stripe-go/invoiceitem"
)

// the minimum value (in cents) a invoice must equal to send off to Stripe
// otherwise it will rollover to the next invoice
const MinimumInvoiceAmount = 100

var (
	ErrNoUsage = errors.New("biller: no usage from user")
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
		}
		return err
	}

	return b.processStripe(i)
}

func (b *Biller) createOrUpdateInvoice() (*InvoiceResult, error) {
	usages, err := hosttransition.BillableHostHours(b.db, b.user.ID, *b.periodStart, *b.periodEnd)
	if err != nil {
		return nil, fmt.Errorf("biller: failed retrieving billable host hours for user %d, err: %v", b.user.ID, err)
	}
	totalCharge, items := InvoiceItemsForUsages(usages)
	i, err := invoice.ForPeriod(b.db, b.user.ID, *b.periodStart, *b.periodEnd)
	if err != nil {
		return nil, fmt.Errorf("biller: encountered error when try to find existing invoice for user %d, err: %v", b.user.ID, err)
	}

	if len(items) == 0 {
		return nil, ErrNoUsage
	}

	tx, err := b.db.Beginx()
	if err != nil {
		return nil, err
	}

	op := i.Update
	isNew := false
	if i == nil {
		isNew = true
		i = &invoice.Invoice{
			UserID:      b.user.ID,
			PeriodStart: b.periodStart,
			PeriodEnd:   b.periodEnd,
			Attempts:    0,
		}
		op = i.Insert
	} else {
		if i.SuccessfulCharge {
			return nil, fmt.Errorf("biller: cannot touch invoice %d, already paid", i.ID)
		}
	}
	i.TotalCharge = totalCharge
	i.SuccessfulCharge = false

	if err := op(tx); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("biller: failure inserting/updating invoice %d for user %d, err: %v", i.ID, i.UserID, err)
	} else {
		b.logger.Infof("biller: created/updated invoice %d for user %d", i.ID, i.UserID)
	}

	deductionCredits, err := takeAvailableCredits(tx, i.UserID, i.ID, i.TotalCharge)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("biller: failure acquiring credits for invoice %d for user %d, err: %v", i.ID, i.UserID, err)
	}

	if deductionCredits < 0 {
		i.TotalCharge += deductionCredits
		i.CreditAmount += (-1 * deductionCredits)
		if err := i.Update(tx); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("biller: failure deducting %d credits from invoice %d for user %d, err: %v", deductionCredits, i.ID, i.UserID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("biller: failure committing invoice %d for user %d, err: %v", i.ID, i.UserID, err)
	}

	items, err = b.applyInvoiceItems(i, isNew, items)
	if err != nil {
		return nil, fmt.Errorf("biller: failure inserting/updating invoice %d items for %d, err: %v", i.ID, i.UserID, err)
	}

	return &InvoiceResult{Invoice: i, Items: items}, nil
}

func (b *Biller) submitRecentlyDeferred() error {
	b.logger.Warnf("biller: looking for deferred invoices to submit for user %d", b.user.ID)

	inv := &invoice.Invoice{}
	err := b.db.Get(inv, `SELECT * FROM invoices WHERE user_id = $1 AND deferred = TRUE ORDER BY id DESC LIMIT 1`,
		b.user.ID)
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
	if !b.user.BillingEnabled {
		b.logger.Warnf("biller: billing disabled for user %d", b.user.ID)
		return nil
	}

	bp, err := billingprofile.Find(b.db, b.user.ID)
	if err != nil || bp == nil || bp.StripeID.String == "" {
		return fmt.Errorf("biller: failure obtaining billing profile for user %d, err: %v", b.user.ID, err)
	}

	if inv.TotalCharge == 0 {
		b.logger.Infof("biller: charge 0, not contacting stripe to charge for items in invoice %d", inv.ID)
	} else {
		b.logger.Infof("biller: contacting stripe for invoice %d", inv.ID)
		if err := b.billStripeInvoiceItems(bp, inv, items); err != nil {
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
			inv.Deferred = true
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

func (b *Biller) billStripeInvoiceItems(bp *billingprofile.BillingProfile, i *invoice.Invoice, items []invoiceitem.InvoiceItem) error {
	hosts, err := host.FindAllWithTerminated(b.db, b.user)
	if err != nil {
		return err
	}
	tx, err := b.db.Beginx()
	if err != nil {
		return err
	}
	for _, invI := range items {
		var matchingHost *host.Host
		for indx, hst := range hosts {
			if hst.ID == invI.HostID {
				matchingHost = &hosts[indx]
				break
			}
		}

		// if this invoice item references a missing host and it is not a credit, raise an error
		if matchingHost == nil && invI.Credit == 0 {
			tx.Rollback()
			return fmt.Errorf("Could not locate host %d for invoice %d", invI.HostID, invI.ID)
		}

		desc := invoiceitem.ItemDescription(&invI, matchingHost)
		if invI.StripeID.String == "" {
			ii, err := striitem.New(&stripe.InvoiceItemParams{
				Customer: bp.StripeID.String,
				Amount:   invI.TotalCharge(),
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
					Amount: invI.TotalCharge(),
					Desc:   desc,
				})
			if err != nil {
				tx.Rollback()
				return err
			}
		}
		b.logger.Infof("biller: submitted invoice item %d", invI.ID)
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
				if inv.HostID == ex.HostID {
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
		if itm.Credit != 0 {
			totalAmountCredited += itm.Credit
		}
	}
	if totalAmountCredited != i.CreditAmount {
		// need to create invoice item to cover difference
		itm := &invoiceitem.InvoiceItem{
			InvoiceID: i.ID,
			Credit:    i.CreditAmount - totalAmountCredited,
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

	stmt := fmt.Sprintf("SELECT DISTINCT ON (invoices.id) invoices.* FROM invoices INNER JOIN invoice_items ON (invoices.id = invoice_items.invoice_id) WHERE invoices.id != $1 AND %s;", strings.Join(itemIDs, " OR "))
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

func takeAvailableCredits(tx *sqlx.Tx, userID int64, invID int64, amount int64) (int64, error) {
	// insert into credits ONLY if the number of available of credits is greater than 0
	// using FOR UPDATE to ensure the rows are locked from other queries modifying
	appliedCredits := int64(0)
	err := tx.QueryRowx(`INSERT INTO credits(user_id, invoice_id, amount)
                  SELECT $1, $2, (-1 * LEAST((SELECT SUM(amount) t FROM (SELECT amount FROM credits WHERE user_id = $1 FOR UPDATE) AS x), $3))
                  WHERE EXISTS (
                    SELECT * FROM (SELECT SUM(amount) t FROM (SELECT amount FROM credits WHERE user_id = $1 FOR UPDATE) AS x) AS x WHERE t > 0
                  )
                  RETURNING amount;`,
		userID, invID, amount).Scan(&appliedCredits)
	if err != nil && err != sql.ErrNoRows {
		return int64(0), err
	}
	return appliedCredits, nil
}

func ProcessForAllUsers(d *db.Database, l *logger.Logger, periodStart, periodEnd *time.Time) {
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

	l.Debugf("biller: Processing period %v to %v", pS, pE)

	lastId := int64(-1)
	usersExhausted := false

	// page through the hosts in the DB, 100 at a time
	for !usersExhausted {
		users := []user.User{}
		err := d.Select(&users, `SELECT * FROM users WHERE id > $1 ORDER BY id ASC LIMIT 100;`, lastId)
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

func InvoiceItemsForUsages(usages []hosttransition.HostHourUsage) (int64, []invoiceitem.InvoiceItem) {
	totalCharge := int64(0)
	items := make([]invoiceitem.InvoiceItem, len(usages))
	for indx, usage := range usages {
		i := invoiceitem.InvoiceItem{
			Hours:       usage.Hours(),
			HourlyPrice: usage.HourlyPrice,
			HostID:      usage.HostID,
		}
		items[indx] = i
		totalCharge += i.TotalCharge()
	}
	return totalCharge, items
}

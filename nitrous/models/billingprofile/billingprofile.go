package billingprofile

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
)

var (
	StripeProfileRequiredError = errors.New("billingprofile: requires an existing Stripe profile")

	ErrCardExpired     = errors.New("billingprofile: card expired")
	ErrCardDeclined    = errors.New("billingprofile: card declined")
	ErrIncorrectCvc    = errors.New("billingprofile: cvc is incorrect")
	ErrIncorrectZipErr = errors.New("billingprofile: zip code is incorrect")
	ErrCardBanned      = errors.New("billingprofile: card banned")
)

const (
	// User with no card added (initial state)
	StateNoCard = "no_card"
	// User exempt from paying
	StateExempt = "exempt"
	// User under free tier
	StateFree = "free"
	// User pays for usage
	StatePaid = "paid"
	// User with payment due
	StatePaymentDue = "payment_due"
	// User who has failed to pay invoices
	StateDelinquent = "delinquent"
)

type BillingProfile struct {
	ID                int64          `db:"id"`
	UserID            int64          `db:"user_id"`
	OrgID             sql.NullInt64  `db:"org_id"`
	State             string         `db:"state"` // possible states: 'free', 'exempt', 'paid', 'payment_due', 'delinquent', 'no_card'
	StripeID          sql.NullString `db:"stripe_id"`
	StripeFingerprint sql.NullString `db:"stripe_fingerprint"`
	LastFour          sql.NullString `db:"last_four"`
	CardType          sql.NullString `db:"card_type"`
	ExpMonth          sql.NullInt64  `db:"exp_month"`
	ExpYear           sql.NullInt64  `db:"exp_year"`
	PendingActivation bool           `db:"pending_activation"` // not activated yet
	FailedAttempts    int            `db:"failed_attempts"`
	MaxHours          sql.NullInt64  `db:"max_hours"`
	CreatedAt         *time.Time     `db:"created_at"`
	UpdatedAt         *time.Time     `db:"updated_at"`
}

func (bp *BillingProfile) IsFreeTier() bool {
	return bp.State == StateFree
}

func (bp *BillingProfile) IsExempt() bool {
	return bp.State == StateExempt
}

func (bp *BillingProfile) MaxMinutes() int64 {
	return bp.MaxHours.Int64 * 60
}

func (bp *BillingProfile) EligibleForSubscription() bool {
	return bp.State == StateFree ||
		bp.State == StatePaid ||
		bp.State == StateExempt ||
		bp.State == StatePaymentDue
}

func (bp *BillingProfile) CanCreateHosts() bool {
	switch bp.State {
	// noop for these cases
	case StatePaid:
	case StatePaymentDue:
	case StateExempt:
	case StateFree:
	default:
		// return disallowed if not in any of previous state
		return false
	}
	return true
}

func FindByID(d db.QueryReceiver, id int64) (*BillingProfile, error) {
	b := &BillingProfile{}
	err := d.Get(b, "SELECT * FROM billing_profiles WHERE id = $1 ORDER BY created_at DESC LIMIT 1;", id)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func FindForOrgs(d db.QueryReceiver, orgIDs []int64, userID int64) ([]*BillingProfile, error) {
	bps := []*BillingProfile{}
	if len(orgIDs) == 0 {
		return bps, nil
	}

	arg := map[string]interface{}{
		"user_id": userID,
		"ids":     orgIDs,
		"roles":   []string{orgmembership.RoleOwner, orgmembership.RoleAdmin},
	}
	query, args, err := sqlx.Named(`SELECT b.* FROM billing_profiles b LEFT JOIN org_memberships o ON o.org_id = b.org_id WHERE b.org_id IN (:ids) AND (b.user_id = :user_id OR (o.role IN (:roles) AND o.user_id = :user_id));`, arg)
	if err != nil {
		return bps, err
	}
	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return bps, err
	}
	query = d.Rebind(query)
	err = d.Select(&bps, query, args...)
	if err != nil {
		return bps, err
	}
	return bps, nil
}

func FindForOrg(d db.QueryReceiver, orgID int64) (*BillingProfile, error) {
	b := &BillingProfile{}
	err := d.Get(b, "SELECT * FROM billing_profiles WHERE org_id = $1 ORDER BY created_at DESC LIMIT 1;", orgID)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func Find(d db.QueryReceiver, userID int64) (*BillingProfile, error) {
	b := &BillingProfile{}
	err := d.Get(b, "SELECT * FROM billing_profiles WHERE user_id = $1 AND org_id IS NULL ORDER BY created_at DESC LIMIT 1;", userID)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func Create(tx *sqlx.Tx, userID int64, orgID sql.NullInt64) error {
	_, err := tx.Exec(`INSERT INTO billing_profiles (user_id, org_id, state) VALUES ($1, $2, $3);`, userID, orgID, StateNoCard)
	return err
}

// Update changes the persisted billing profile record.
// To update the card or email in Stripe, use UpdateCard or UpdateEmail.
func (b *BillingProfile) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE billing_profiles
		SET (stripe_id, stripe_fingerprint, last_four, card_type, exp_month, exp_year, state, pending_activation, max_hours, updated_at)
		= (:stripe_id, :stripe_fingerprint, :last_four, :card_type, :exp_month, :exp_year, :state, :pending_activation, :max_hours, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(b).StructScan(b)
}

func (b *BillingProfile) UpdateCard(d *db.Database, token string) error {
	if !b.StripeID.Valid {
		return StripeProfileRequiredError
	}

	params := &stripe.CustomerParams{
		Token: token,
	}
	c, err := customer.Update(b.StripeID.String, params)
	if err != nil {
		return HandleStripeError(err)
	}

	crd := c.Cards.Values[0]

	if err := b.SetCard(d, crd); err != nil {
		return err
	}
	if b.State == StateNoCard || b.State == StateFree {
		b.State = StatePaid
	}

	tx, err := d.Beginx()
	if err != nil {
		return err
	}

	if err := b.Update(tx); err != nil {
		tx.Rollback()
		return fmt.Errorf("Billing profile update failed. Reason - %s", err)
	}

	return tx.Commit()
}

func (b *BillingProfile) SetCard(d *db.Database, crd *stripe.Card) error {
	var banned bool
	err := d.Get(&banned, `SELECT COUNT(*) > 0
		FROM billing_profiles
		INNER JOIN users ON users.id = billing_profiles.user_id
		WHERE banned_at IS NOT NULL AND stripe_fingerprint = $1 LIMIT 1;`, crd.Fingerprint)
	if err != nil {
		return err
	}
	if banned {
		return ErrCardBanned
	}

	b.LastFour = sql.NullString{crd.LastFour, true}
	b.CardType = sql.NullString{string(crd.Brand), true}
	b.ExpMonth = sql.NullInt64{int64(crd.Month), true}
	b.ExpYear = sql.NullInt64{int64(crd.Year), true}
	b.StripeFingerprint = sql.NullString{crd.Fingerprint, true}
	b.PendingActivation = false
	b.State = StatePaid

	return nil
}

func (b *BillingProfile) Activate(d *db.Database, cus *stripe.Customer) error {
	b.StripeID = sql.NullString{cus.ID, true}
	b.PendingActivation = false

	if cus.Cards != nil && len(cus.Cards.Values) >= 1 {
		if err := b.SetCard(d, cus.Cards.Values[0]); err != nil {
			return err
		}
	}

	return d.WithTransaction(func(t *sqlx.Tx) error {
		if err := b.Update(t); err != nil {
			return fmt.Errorf("Billing profile update failed for user %d org %d. Reason - %s", b.UserID, b.OrgID.Int64, err)
		}
		return nil
	})
}

func (b *BillingProfile) UpdateEmail(d db.QueryReceiver, email string) error {
	if !b.StripeID.Valid {
		return StripeProfileRequiredError
	}

	params := &stripe.CustomerParams{
		Email: email,
	}
	if _, err := customer.Update(b.StripeID.String, params); err != nil {
		return err
	}

	return nil
}

func (p *BillingProfile) IsCardRequired() bool {
	return p.State == StateNoCard
}

func (p *BillingProfile) HasBillingIssues() bool {
	return p.State == StateDelinquent
}

func (p *BillingProfile) IsFreeOrCardRequired() bool {
	return p.IsFreeTier() || p.IsCardRequired()
}

func (p *BillingProfile) MaxAttempts() bool {
	// There is an automatic retry on every attempt by the user to add a new card. So we give the user a maximum of 2 tries to add a new card.
	return p.FailedAttempts == 4
}

func HandleStripeError(err error) error {
	if e, ok := err.(*stripe.Error); ok {
		if e.Code == stripe.CardDeclined {
			return ErrCardDeclined
		} else if e.Code == stripe.ExpiredCard {
			return ErrCardExpired
		} else if e.Code == stripe.IncorrectCvc {
			return ErrIncorrectCvc
		} else if e.Code == stripe.IncorrectZip {
			return ErrIncorrectZipErr
		}
	}
	return err
}

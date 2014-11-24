package billingprofile

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
)

func init() {
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	if stripe.Key == "" {
		panic("STRIPE_SECRET_KEY environment variable not set")
	}
}

const (
	StateFree       = "free"
	StatePaid       = "paid"
	StatePaymentDue = "payment_due"
	StateDelinquent = "delinquent"
)

type BillingProfile struct {
	ID        int64          `db:"id"`
	UserID    int64          `db:"user_id"`
	State     string         `db:"state"` // possible states: 'free', 'paid', 'payment_due', 'delinquent'
	StripeID  sql.NullString `db:"stripe_id"`
	LastFour  sql.NullString `db:"last_four"`
	CardType  sql.NullString `db:"card_type"`
	ExpMonth  sql.NullInt64  `db:"exp_month"`
	ExpYear   sql.NullInt64  `db:"exp_year"`
	CreatedAt *time.Time     `db:"created_at"`
	UpdatedAt *time.Time     `db:"updated_at"`
}

func Find(d db.QueryReceiver, userID int64) (*BillingProfile, error) {
	b := &BillingProfile{}
	err := d.Get(b, "SELECT * FROM billing_profiles WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1;", userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

func Create(tx *sqlx.Tx, userID int64) error {
	_, err := tx.Exec(`INSERT INTO billing_profiles (user_id, state) VALUES ($1, $2);`, userID, StateFree)
	return err
}

// Update changes the persisted billing profile record.
// To update the card or email in Stripe, use UpdateCard or UpdateEmail.
func (b *BillingProfile) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE billing_profiles 
		SET (stripe_id, last_four, card_type, exp_month, exp_year, state, updated_at)
		= (:stripe_id, :last_four, :card_type, :exp_month, :exp_year, :state, now())
		WHERE user_id = :user_id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(b).StructScan(b)
}

func (b *BillingProfile) UpdateCard(d db.QueryReceiver, token string) error {
	if !b.StripeID.Valid {
		return fmt.Errorf("UpdateCard: requires an existing Stripe profile")
	}

	params := &stripe.CustomerParams{
		Token: token,
	}
	c, err := customer.Update(b.StripeID.String, params)
	if err != nil {
		return fmt.Errorf("Stripe couldn't update the card of profile %s. Reason - %s", b.StripeID, err)
	}

	crd := c.Cards.Values[0]
	b.StripeID = sql.NullString{c.ID, true}
	b.LastFour = sql.NullString{crd.LastFour, true}
	b.CardType = sql.NullString{string(crd.Brand), true}
	b.ExpMonth = sql.NullInt64{int64(crd.Month), true}
	b.ExpYear = sql.NullInt64{int64(crd.Year), true}

	if err := b.Update(d); err != nil {
		return fmt.Errorf("Billing profile update failed. Reason - %s", err)
	}
	return nil
}

func (b *BillingProfile) UpdateEmail(d db.QueryReceiver, email string) error {
	if !b.StripeID.Valid {
		return fmt.Errorf("UpdateEmail: requires an existing Stripe profile")
	}

	params := &stripe.CustomerParams{
		Email: email,
	}
	if _, err := customer.Update(b.StripeID.String, params); err != nil {
		return fmt.Errorf("Stripe couldn't update the card of profile %s. Reason - %s", b.StripeID, err)
	}

	return nil
}

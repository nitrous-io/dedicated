package billingprofile

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
)

var (
	StripeProfileRequiredError = errors.New("billingprofile: requires an existing Stripe profile")

	CardExpiredError  = errors.New("billingprofile: card expired")
	CardDeclinedError = errors.New("billingprofile: card declined")
	IncorrectCvcError = errors.New("billingprofile: cvc is incorrect")
	IncorrectZipError = errors.New("billingprofile: zip code is incorrect")
)

const (
	StateFree       = "free"
	StatePaid       = "paid"
	StatePaymentDue = "payment_due"
	StateDelinquent = "delinquent"
)

type BillingProfile struct {
	ID                int64          `db:"id"`
	UserID            int64          `db:"user_id"`
	State             string         `db:"state"` // possible states: 'free', 'paid', 'payment_due', 'delinquent'
	StripeID          sql.NullString `db:"stripe_id"`
	StripeFingerprint sql.NullString `db:"stripe_fingerprint"`
	LastFour          sql.NullString `db:"last_four"`
	CardType          sql.NullString `db:"card_type"`
	ExpMonth          sql.NullInt64  `db:"exp_month"`
	ExpYear           sql.NullInt64  `db:"exp_year"`
	CreatedAt         *time.Time     `db:"created_at"`
	UpdatedAt         *time.Time     `db:"updated_at"`
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
		SET (stripe_id, stripe_fingerprint, last_four, card_type, exp_month, exp_year, state, updated_at)
		= (:stripe_id, :stripe_fingerprint, :last_four, :card_type, :exp_month, :exp_year, :state, now())
		WHERE user_id = :user_id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(b).StructScan(b)
}

func (b *BillingProfile) UpdateCard(d db.QueryReceiver, token string) error {
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
	b.StripeID = sql.NullString{c.ID, true}
	b.StripeFingerprint = sql.NullString{crd.Fingerprint, true}
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

func HandleStripeError(err error) error {
	if e, ok := err.(*stripe.Error); ok {
		if e.Code == stripe.CardDeclined {
			return CardDeclinedError
		} else if e.Code == stripe.ExpiredCard {
			return CardExpiredError
		} else if e.Code == stripe.IncorrectCvc {
			return IncorrectCvcError
		} else if e.Code == stripe.IncorrectZip {
			return IncorrectZipError
		}
	}
	return err
}

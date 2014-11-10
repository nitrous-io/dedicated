package billingprofile

import (
	"database/sql"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/stripe/stripe-go"
)

func init() {
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
	if stripe.Key == "" {
		panic("STRIPE_SECRET_KEY environment variable not set")
	}
}

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
	_, err := tx.Exec(`INSERT INTO billing_profiles (user_id, state) VALUES ($1, 'free');`, userID)
	return err
}

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

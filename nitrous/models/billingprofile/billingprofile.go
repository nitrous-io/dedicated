package billingprofile

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
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

func (b *BillingProfile) insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO billing_profiles 
		(user_id, stripe_id, last_four, card_type, exp_month, exp_year, state)
		VALUES (:user_id, :stripe_id, :last_four, :card_type, :exp_month, :exp_year, :state)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(b).StructScan(b)
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

func Create(d db.QueryReceiver, u *user.User, token string) (*BillingProfile, error) {
	if u == nil {
		return nil, errors.New("user not found")
	}

	params := &stripe.CustomerParams{
		Token: token,
		Email: u.Email,
		Desc:  fmt.Sprint(u.Name, u.Username),
	}
	c, err := customer.New(params)
	if err != nil {
		return nil, err
	}

	// persist the profile
	crd := c.Cards.Values[0]
	b := &BillingProfile{
		UserID:   u.ID,
		StripeID: c.ID,
		LastFour: crd.LastFour,
		CardType: string(crd.Brand),
		ExpMonth: crd.Month,
		ExpYear:  crd.Year,
	}

	err = b.insert(d)
	if err != nil {
		return nil, err
	}
	return b, nil
}

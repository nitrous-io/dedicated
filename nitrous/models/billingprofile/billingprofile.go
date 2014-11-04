package billingprofile

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
)

type BillingProfile struct {
	ID        int64      `db:"id"`
	UserID    int64      `db:"user_id"`
	StripeID  string     `db:"stripe_id"`
	LastFour  string     `db:"last_four"`
	CardType  string     `db:"card_type"`
	ExpMonth  uint8      `db:"exp_month"`
	ExpYear   uint16     `db:"exp_year"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`

	Token string // credit card token (used by Stripe to identify customer's card)
}

func (b *BillingProfile) insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO billing_profiles 
		(user_id, stripe_id, last_four, card_type, exp_month, exp_year)
		VALUES (:user_id, :stripe_id, :last_four, :card_type, :exp_month, :exp_year)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(b).StructScan(b)
}

func Find(d db.QueryReceiver, u *user.User) (*BillingProfile, error) {
	b := &BillingProfile{}
	err := d.Get(b, "SELECT * FROM billing_profiles WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1;", u.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

func (b *BillingProfile) Create(d db.QueryReceiver) error {
	u, err := user.FindByID(d, b.UserID)
	if err != nil {
		return err
	}
	if u == nil {
		return errors.New("user not found")
	}

	params := &stripe.CustomerParams{
		Token: b.Token,
		Email: u.Email,
		Desc:  fmt.Sprint(u.Name, u.Username),
	}
	c, err := customer.New(params)
	if err != nil {
		return err
	}

	// insert to DB
	b.StripeID = c.ID

	crd := c.DefaultCard
	b.LastFour = crd.LastFour
	b.CardType = string(crd.Brand)
	b.ExpMonth = crd.Month
	b.ExpYear = crd.Year
	return b.insert(d)
}

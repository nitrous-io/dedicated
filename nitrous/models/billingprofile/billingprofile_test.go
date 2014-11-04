package billingprofile_test

import (
	"fmt"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stripe/stripe-go"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/billingprofile")
}

var _ = Describe("models/billingprofile", func() {
	var (
		db         *db.Database
		u          *user.User
		fakeStripe *FakeStripe
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		stripe.Key = "sk_key"
		fakeStripe = &FakeStripe{}
		stripe.SetBackend(fakeStripe)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Create", func() {
		It("returns an error when user is nil", func() {
			b := &billingprofile.BillingProfile{}
			b.UserID = 9999
			b.Token = "token"

			// return an error
			Expect(b.Create(db)).NotTo(BeNil())
		})

		It("sends a request to Stripe to create a customer", func() {
			fakeStripe.Response = fmt.Sprintf(`{
					"id": "sc_01",
					"email": "%s",
					"default_card": {
						"exp_month": 10,
						"exp_year": 2018,
						"brand": "Visa",
						"last4": "4242"
					}
				}`, u.Email)

			b := &billingprofile.BillingProfile{}
			b.UserID = u.ID
			b.Token = "token"
			err := b.Create(db)
			Expect(err).To(BeNil())

			r := fakeStripe.Received
			Expect(r.Method).To(Equal("POST"))
			Expect(r.Path).To(Equal("/customers"))
			Expect(r.Card).To(Equal("token"))
			Expect(r.Email).To(Equal(u.Email))
			Expect(r.Desc).To(Equal(fmt.Sprint(u.Name, u.Username)))
		})

		Context("when Stripe returns the created customer", func() {
			It("updates and persists the record", func() {
				fakeStripe.Response = fmt.Sprintf(`{
						"id": "sc_01",
						"email": "%s",
						"default_card": {
							"exp_month": 10,
							"exp_year": 2018,
							"brand": "Visa",
							"last4": "4242"
						}
					}`, u.Email)

				b := &billingprofile.BillingProfile{}
				b.UserID = u.ID
				b.Token = "token"
				err := b.Create(db)
				Expect(err).To(BeNil())

				Expect(b.StripeID).To(Equal("sc_01"))
				Expect(b.LastFour).To(Equal("4242"))
				Expect(b.CardType).To(Equal("Visa"))
				Expect(b.ExpMonth).To(Equal(uint8(10)))
				Expect(b.ExpYear).To(Equal(uint16(2018)))

				b, err = billingprofile.Find(db, u)
				Expect(err).To(BeNil())
				Expect(b.ID).NotTo(BeNil())
				Expect(b.UserID).To(Equal(u.ID))
				Expect(b.StripeID).To(Equal("sc_01"))
				Expect(b.LastFour).To(Equal("4242"))
				Expect(b.CardType).To(Equal("Visa"))
				Expect(b.ExpMonth).To(Equal(uint8(10)))
				Expect(b.ExpYear).To(Equal(uint16(2018)))
			})
		})

		Context("when Stripe returns an error", func() {
			It("returns the error, without persisting the record", func() {
				fakeStripe.Error = &stripe.Error{
					Type: stripe.ErrorType(stripe.APIErr),
					Msg:  "some error occurred",
					Code: stripe.ErrorCode(stripe.ProcessingErr),
				}

				b := &billingprofile.BillingProfile{}
				b.UserID = u.ID
				b.Token = "token"
				err := b.Create(db)
				Expect(err).NotTo(BeNil())

				Expect(b.StripeID).To(BeEmpty())
				Expect(b.LastFour).To(BeEmpty())
				Expect(b.CardType).To(BeEmpty())
				Expect(b.ExpMonth).To(BeZero())
				Expect(b.ExpYear).To(BeZero())

				b, err = billingprofile.Find(db, u)
				Expect(err).To(BeNil())
				Expect(b).To(BeNil())
			})
		})
	})
})

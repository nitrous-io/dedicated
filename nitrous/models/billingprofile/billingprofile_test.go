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
			b, err := billingprofile.Create(db, nil, "token")
			Expect(b).To(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("sends a request to Stripe to create a customer", func() {
			billingprofile.Create(db, u, "token")

			Expect(fakeStripe.Received).To(Equal(map[string]string{
				"method": "POST",
				"path":   "/customers",
				"card":   "token",
				"email":  u.Email,
				"desc":   fmt.Sprint(u.Name, u.Username),
			}))
		})

		Context("when Stripe returns the created customer", func() {
			It("updates and persists the record", func() {
				fakeStripe.Response = fmt.Sprintf(`{
					"id": "sc_01",
					"email": "%s",
					"cards": {
						"object": "list",
						"total_count": 1,
						"has_more": false,
						"url": "/v1/customers/cus_56dgsyZAJujFFO/cards",
						"data": [
							{
								"id": "card_14wQPX2CU2PklWtHi34KFonT",
								"object": "card",
								"last4": "4242",
								"brand": "Visa",
								"funding": "credit",
								"exp_month": 10,
								"exp_year": 2018,
								"fingerprint": "H0SJieNrXmTdq3qs",
								"country": "US",
								"name": null,
								"address_line1": null,
								"address_line2": null,
								"address_city": null,
								"address_state": null,
								"address_zip": null,
								"address_country": null,
								"cvc_check": "pass",
								"address_line1_check": null,
								"address_zip_check": null,
								"dynamic_last4": null,
								"customer": "cus_56dgsyZAJujFFO"
							}
						]
					},
					"default_card": "card_14wQPX2CU2PklWtHi34KFonT"
				}`, u.Email)

				b, err := billingprofile.Create(db, u, "token")
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

				_, err := billingprofile.Create(db, u, "token")
				Expect(err).NotTo(BeNil())

				b, err := billingprofile.Find(db, u)
				Expect(err).To(BeNil())
				Expect(b).To(BeNil())
			})
		})
	})
})

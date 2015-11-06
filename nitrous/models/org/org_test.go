package org_test

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/plan_segment"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	stripe "github.com/stripe/stripe-go"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/org")
}

var _ = Describe("models/org", func() {
	var (
		db             *db.Database
		o1, o2, o3, o4 *org.Org
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		o1 = &org.Org{
			Name:  "Foo Bar, Inc.",
			Email: "eng@foobar.com",
			Slug:  "foobar",
		}
		Expect(o1.Insert(db)).To(BeNil())

		o2 = &org.Org{
			Name:  "Watermelon Corp.",
			Email: "admin@watermelon.co",
			Slug:  "watermelon",
		}
		Expect(o2.Insert(db)).To(BeNil())

		o3 = &org.Org{
			Name:  "Macrosoft Corp.",
			Email: "bill@macrosoft.com",
			Slug:  "macrosoft",
		}
		Expect(o3.Insert(db)).To(BeNil())

		o4 = &org.Org{
			Name:  "Action Enterprises",
			Email: "dev@actionent.io",
			Slug:  "action-enterprises",
		}
		Expect(o4.Insert(db)).To(BeNil())

		_, err = db.Exec("UPDATE orgs SET deleted_at = now() WHERE id = $1", o4.ID)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindBySlug()", func() {
		Context("when a record with a given slug exists", func() {
			It("returns the record", func() {
				o, err := org.FindBySlug(db, "foobar")
				Expect(err).To(BeNil())
				Expect(o).NotTo(BeNil())
				Expect(o.ID).To(Equal(o1.ID))
				Expect(o.Name).To(Equal("Foo Bar, Inc."))
				Expect(o.Slug).To(Equal("foobar"))
				Expect(o.Email).To(Equal("eng@foobar.com"))

				o2, err := org.FindBySlug(db, "watermelon")
				Expect(err).To(BeNil())
				Expect(o2).NotTo(BeNil())
				Expect(o2).NotTo(Equal(o))
				Expect(o2.ID).To(Equal(o2.ID))
				Expect(o2.Name).To(Equal("Watermelon Corp."))
				Expect(o2.Slug).To(Equal("watermelon"))
				Expect(o2.Email).To(Equal("admin@watermelon.co"))
			})
		})

		Context("when a record with a given slug does not exist", func() {
			It("returns error", func() {
				o, err := org.FindBySlug(db, "acme")
				Expect(o).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a record with a given slug is deleted", func() {
			It("returns error", func() {
				o, err := org.FindBySlug(db, "action-enterprise")
				Expect(o).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				o, err := org.FindBySlug(db, "foobar")
				Expect(o).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindByIdOrSlug()", func() {
		It("returns the record with given slug or ID", func() {
			o, err := org.FindByIdOrSlug(db, "foobar")
			Expect(err).To(BeNil())
			Expect(o.ID).To(Equal(o1.ID))
			Expect(o.Name).To(Equal("Foo Bar, Inc."))
			Expect(o.Slug).To(Equal("foobar"))
			Expect(o.Email).To(Equal("eng@foobar.com"))

			id := strconv.FormatInt(o1.ID, 10)
			o, err = org.FindByIdOrSlug(db, id)
			Expect(err).To(BeNil())
			Expect(o.ID).To(Equal(o1.ID))
			Expect(o.Slug).To(Equal("foobar"))
			Expect(o.Name).To(Equal("Foo Bar, Inc."))
			Expect(o.Slug).To(Equal("foobar"))
			Expect(o.Email).To(Equal("eng@foobar.com"))
		})
	})

	Describe("Find()", func() {
		Context("when a record with a given ID exists", func() {
			It("returns the record", func() {
				o, err := org.Find(db, o1.ID)
				Expect(err).To(BeNil())
				Expect(o).NotTo(BeNil())
				Expect(o.Name).To(Equal("Foo Bar, Inc."))
				Expect(o.Slug).To(Equal("foobar"))
				Expect(o.Email).To(Equal("eng@foobar.com"))

				o2, err := org.Find(db, o2.ID)
				Expect(err).To(BeNil())
				Expect(o2).NotTo(BeNil())
				Expect(o2).NotTo(Equal(o))
				Expect(o2.Name).To(Equal("Watermelon Corp."))
				Expect(o2.Slug).To(Equal("watermelon"))
				Expect(o2.Email).To(Equal("admin@watermelon.co"))
			})
		})

		Context("when a record with a given org does not exist", func() {
			It("returns error", func() {
				o, err := org.Find(db, -99)
				Expect(o).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when a record with a given org is deleted", func() {
			It("returns error", func() {
				o, err := org.Find(db, o4.ID)
				Expect(o).To(BeNil())
				Expect(err).To(MatchError(sql.ErrNoRows))
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				o, err := org.Find(db, o1.ID)
				Expect(o).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var o *org.Org

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Pineapple, Inc.",
				Slug:  "pineapple-inc",
				Email: "tcook@pineapple.com",
			}
		})

		Context("when no error happens", func() {
			It("persists the org in the database", func() {
				err := o.Insert(db)
				Expect(err).To(BeNil())

				o2, err := org.FindBySlug(db, "pineapple-inc")
				Expect(err).To(BeNil())
				Expect(o2).NotTo(BeNil())
				Expect(o2.Name).To(Equal("Pineapple, Inc."))
				Expect(o2.Slug).To(Equal("pineapple-inc"))
				Expect(o2.Email).To(Equal("tcook@pineapple.com"))
				Expect(o2.CreatedAt).NotTo(BeNil())
				Expect(o2.UpdatedAt).NotTo(BeNil())
			})
		})

		Context("when slug is taken", func() {
			It("returns error", func() {
				o.Slug = "foobar"
				err := o.Insert(db)
				Expect(err).NotTo(BeNil())
			})
		})

		Context("when some error occurs", func() {
			It("returns error", func() {
				db.Close()
				err := o.Insert(db)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Validate()", func() {
		var o *org.Org

		BeforeEach(func() {
			o = &org.Org{
				Name:  "Macrosoft Corp.",
				Slug:  "macrosoft",
				Email: "bill@macrosoft.com",
			}
			Expect(o.Validate()).To(BeNil())
		})

		It("validates email", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", org.ErrEmailBlank},
				{"foo", org.ErrEmailInvalid},
				{"foo@gmail.com", nil},
				{"foo+bar@gmail.com", nil},
				{"foo+bar@hello.com.sg", nil},
			}

			for _, t := range tests {
				o.Email = t.input
				Expect(o.Validate() == t.err).To(BeTrue())
			}
		})

		It("validates email with email blacklist", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"foo@yopmail.com", org.ErrEmailBlacklisted},
				{"foo@yopmail.fr", org.ErrEmailBlacklisted},
				{"foo@gmail.com", nil},
			}

			for _, t := range tests {
				o.Email = t.input
				Expect(o.Validate() == t.err).To(BeTrue())
			}
		})

		It("validates slug", func() {
			buf := make([]byte, 64)
			for i := range buf {
				buf[i] = 'f'
			}
			s := string(buf) // "f" * 64.

			tests := []struct {
				input string
				err   error
			}{
				{"", org.ErrSlugBlank},
				{"foo", nil},
				{"fo", nil},
				{"f", org.ErrSlugTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{s, org.ErrSlugTooLong},
				{s[:63], nil},
				{"f00", nil},
				{"f-o-0", nil},
				{"f-o-0-", org.ErrSlugInvalid},
				{"-foo", org.ErrSlugInvalid},
				{"foo_bar", org.ErrSlugInvalid},
				{"FOO", org.ErrSlugInvalid},
				{"123", org.ErrSlugInvalid},
				{"123-foo", nil},
				{"1-foo-2", nil},
			}

			for _, t := range tests {
				o.Slug = t.input
				Expect(o.Validate() == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})

	Describe("FindAllByMemberUserID", func() {
		var (
			oID, o2ID, o3ID, o4ID int64
			u                     *user.User
		)

		BeforeEach(func() {
			err := db.QueryRow("SELECT id FROM orgs WHERE slug = $1", "foobar").Scan(&oID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM orgs WHERE slug = $1", "watermelon").Scan(&o2ID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM orgs WHERE slug = $1", "macrosoft").Scan(&o3ID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM orgs WHERE slug = $1", "action-enterprises").Scan(&o4ID)
			Expect(err).To(BeNil())

			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
				Name:     "Mr. Foo",
			}
			err = test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())
		})

		Context("when a non-existent user id is given", func() {
			It("returns error", func() {
				o, err := org.FindAllByMemberUserID(db, u.ID+9000)
				Expect(err).To(BeNil())
				Expect(o).To(HaveLen(0))
			})
		})

		Context("when a user with given user id exists", func() {
			Context("when user does not belong to any org", func() {
				It("returns error", func() {
					o, err := org.FindAllByMemberUserID(db, u.ID)
					Expect(err).To(BeNil())
					Expect(o).To(HaveLen(0))
				})
			})

			Context("when user belongs to orgs", func() {
				BeforeEach(func() {
					err := orgmembership.AddMember(db, oID, u.ID, "admin")
					Expect(err).To(BeNil())
					err = orgmembership.AddMember(db, o3ID, u.ID, "member")
					Expect(err).To(BeNil())
					err = orgmembership.AddMember(db, o4ID, u.ID, "owner")
					Expect(err).To(BeNil())
				})

				It("returns a list of orgs, excluding deleted orgs", func() {
					o, err := org.FindAllByMemberUserID(db, u.ID)
					Expect(err).To(BeNil())
					Expect(o).To(HaveLen(2))
					Expect(o[0].ID).To(Equal(oID))
					Expect(o[0].Name).To(Equal("Foo Bar, Inc."))
					Expect(o[0].Email).To(Equal("eng@foobar.com"))
					Expect(o[0].Slug).To(Equal("foobar"))
					Expect(o[1].ID).To(Equal(o3ID))
					Expect(o[1].Name).To(Equal("Macrosoft Corp."))
					Expect(o[1].Email).To(Equal("bill@macrosoft.com"))
					Expect(o[1].Slug).To(Equal("macrosoft"))
				})
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				o, err := org.FindAllByMemberUserID(db, u.ID)
				Expect(o).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("CanCreateHosts", func() {
		var (
			u  *user.User
			bp *billingprofile.BillingProfile
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

			err := db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o1.ID, true})).To(BeNil())
				return nil
			})
			Expect(err).To(BeNil())

			bp, err = billingprofile.FindForOrg(db, o1.ID)
			Expect(err).To(BeNil())

			bp.State = billingprofile.StatePaid
			bp.StripeID = sql.NullString{"cus_1x23a", true}
			Expect(bp.Update(db)).To(BeNil())
		})

		It("should return no credit card error if the BP is in a no card state", func() {
			bp.State = billingprofile.StateNoCard
			Expect(bp.Update(db)).To(BeNil())

			err := o1.CanCreateHosts(db)
			Expect(err.Error()).To(Equal("has not added credit card"))
		})

		It("should return unpaid invoices error if the BP is in a delinquent state", func() {
			bp.State = billingprofile.StateDelinquent
			Expect(bp.Update(db)).To(BeNil())

			err := o1.CanCreateHosts(db)
			Expect(err.Error()).To(Equal("outstanding invoices"))
		})

		It("not return error if the BP is in a valid state", func() {
			states := []string{
				billingprofile.StateExempt,
				billingprofile.StatePaid,
				billingprofile.StatePaid,
				billingprofile.StatePaymentDue,
			}
			for _, st := range states {
				bp.State = st
				Expect(bp.Update(db)).To(BeNil())

				err := o1.CanCreateHosts(db)
				Expect(err).To(BeNil())
			}
		})

		It("should return max hosts error if the org exceeds the max hosts", func() {
			_, err := db.Exec(`UPDATE orgs SET max_hosts = 1 WHERE id = $1;`, o1.ID)
			Expect(err).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u,
				O: o1,
			})
			Expect(err).To(BeNil())

			err = o1.CanCreateHosts(db)
			Expect(err.Error()).To(Equal("reached maximum number of hosts"))
		})

		It("not return error if the org does not exceed the max hosts", func() {
			_, err := db.Exec(`UPDATE orgs SET max_hosts = 2 WHERE id = $1;`, o1.ID)
			Expect(err).To(BeNil())

			_, err = test.CreateHostStack(db, &test.HostStack{
				U: u,
				O: o1,
			})
			Expect(err).To(BeNil())

			err = o1.CanCreateHosts(db)
			Expect(err).To(BeNil())
		})
	})

	Describe("ActivateBillingProfile", func() {
		var (
			u          *user.User
			fakeStripe *test.FakeStripe
			bp         *billingprofile.BillingProfile
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

			err := db.WithTransaction(func(t *sqlx.Tx) error {
				Expect(billingprofile.Create(t, u.ID, sql.NullInt64{o1.ID, true})).To(BeNil())
				return nil
			})
			Expect(err).To(BeNil())

			bp, err = billingprofile.FindForOrg(db, o1.ID)
			Expect(err).To(BeNil())
			fakeStripe = &test.FakeStripe{}
			stripe.SetBackend(fakeStripe)
		})

		It("sends a request to Stripe to create a customer", func() {
			o1.ActivateBillingProfile(db, bp, "token")

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			Expect(fakeStripe.ReceivedRequests[0]).To(Equal(map[string]string{
				"method":         "POST",
				"path":           "/customers",
				"card":           "token",
				"email":          o1.Email,
				"desc":           `Organization "Foo Bar, Inc."`,
				"metadata[name]": "Foo Bar, Inc.",
			}))
		})

		It("should send a request if a token is not provided", func() {
			o1.ActivateBillingProfile(db, bp, "")

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			Expect(fakeStripe.ReceivedRequests[0]).To(Equal(map[string]string{
				"method":         "POST",
				"path":           "/customers",
				"card":           "",
				"email":          o1.Email,
				"desc":           `Organization "Foo Bar, Inc."`,
				"metadata[name]": "Foo Bar, Inc.",
			}))
		})

		Context("when Stripe returns the created customer", func() {
			It("updates and persists the record", func() {
				fakeStripe.Responses = []string{test.StripeCustomerResponse}

				err := o1.ActivateBillingProfile(db, bp, "token")
				Expect(err).To(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

				Expect(bp.ID).NotTo(BeNil())
				Expect(bp.UserID).To(Equal(u.ID))
				Expect(bp.StripeID.String).To(Equal("sc_01"))
				Expect(bp.LastFour.String).To(Equal("4242"))
				Expect(bp.CardType.String).To(Equal("Visa"))
				Expect(bp.ExpMonth.Int64).To(Equal(int64(10)))
				Expect(bp.ExpYear.Int64).To(Equal(int64(2018)))
				Expect(bp.PendingActivation).To(BeFalse())
			})

			It("should create a record without a card", func() {
				resp := `{
					"id": "sc_01",
					"email": "user@example.com",
					"cards": null,
					"default_card": "card_14wQPX2CU2PklWtHi34KFonT"
				}`
				fakeStripe.Responses = []string{resp}

				err := o1.ActivateBillingProfile(db, bp, "token")
				Expect(err).To(BeNil())

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))

				Expect(bp.ID).NotTo(BeNil())
				Expect(bp.UserID).To(Equal(u.ID))
				Expect(bp.StripeID.String).To(Equal("sc_01"))
				Expect(bp.LastFour.String).To(BeEmpty())
				Expect(bp.CardType.String).To(BeEmpty())
				Expect(bp.ExpMonth.Int64).To(BeZero())
				Expect(bp.ExpYear.Int64).To(BeZero())
				Expect(bp.State).To(Equal("no_card"))
				Expect(bp.PendingActivation).To(BeFalse())
			})

			It("should not allow a banned card", func() {
				b := &user.User{
					Username: "banner",
					Password: "123456",
					Email:    "banner@example.com",
				}
				err := test.WrapInTransaction(db, b.Insert)
				Expect(err).To(BeNil())

				u2 := &user.User{
					Username: "bad",
					Name:     "quxboy",
					Password: "123456",
					Email:    "bad@example.com",
				}
				err = test.WrapInTransaction(db, u2.Insert)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE users SET deleted_at = now(), banned_at = now(), banned_by_id = $1 WHERE id = $2;`, b.ID, u2.ID)
				Expect(err).To(BeNil())

				_, err = db.Exec(`UPDATE billing_profiles SET stripe_fingerprint = 'H0SJieNrXmTdq3qs' WHERE user_id = $1;`, u2.ID)
				Expect(err).To(BeNil())

				fakeStripe.Responses = []string{test.StripeCustomerResponse}

				err = o1.ActivateBillingProfile(db, bp, "token")
				Expect(err).To(Equal(billingprofile.ErrCardBanned))

				Expect(fakeStripe.ReceivedRequests).To(HaveLen(2))
				delReq := fakeStripe.ReceivedRequests[1]
				Expect(delReq["method"]).To(Equal("DELETE"))
				Expect(delReq["path"]).To(Equal("/customers/sc_01"))
			})
		})

		Context("when Stripe returns a card declined error", func() {
			It("returns the error, without updating the record", func() {
				fakeStripe.Errors = []*stripe.Error{test.StripeCardDeclined}

				err := o1.ActivateBillingProfile(db, bp, "token")
				Expect(err).To(Equal(billingprofile.ErrCardDeclined))
				Expect(bp.State).To(Equal("no_card"))
			})
		})

		Context("when Stripe returns an error", func() {
			It("returns the error, without updating the record", func() {
				fakeStripe.Errors = []*stripe.Error{
					&stripe.Error{
						Type: stripe.ErrorType(stripe.APIErr),
						Msg:  "some error occurred",
						Code: stripe.ErrorCode(stripe.ProcessingErr),
					},
				}

				err := o1.ActivateBillingProfile(db, bp, "token")
				Expect(err).NotTo(BeNil())
				Expect(bp.State).To(Equal("no_card"))
			})
		})
	})

	Describe("Plans", func() {
		var (
			orgDefaultSegmentID int64
			o                   *org.Org
		)

		BeforeEach(func() {
			plans := []*plan.Plan{
				{Slug: "plus", Price: 6400},
				{Slug: "pro", Price: 12800},
			}

			// Get default segment ID
			err := db.QueryRow(`SELECT id from segments where org_default='t'`).Scan(&orgDefaultSegmentID)
			Expect(err).NotTo(HaveOccurred())

			for _, pl := range plans {
				Expect(pl.Insert(db)).To(BeNil())

				ps := plan_segment.PlanSegment{SegmentID: orgDefaultSegmentID, PlanID: pl.ID}
				Expect(ps.Insert(db)).NotTo(HaveOccurred())
			}

			// Add org
			o = &org.Org{
				Name:      "Test",
				Email:     "test@nitrous.io",
				Slug:      "test_org",
				SegmentID: orgDefaultSegmentID,
			}
			Expect(o.Insert(db)).To(BeNil())
		})

		It("returns org plans", func() {
			plans, err := o.Plans(db)
			Expect(err).NotTo(HaveOccurred())

			Expect(plans).To(HaveLen(2))
			Expect(plans[0].Slug).To(Equal("plus"))
			Expect(plans[1].Slug).To(Equal("pro"))
		})
	})
})

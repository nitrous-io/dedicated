package subscription_test

import (
	"database/sql"
	"fmt"
	"time"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("models/subscription", func() {
	var (
		db *ndb.Database
		u  *user.User
		pl *plan.Plan
		hs *test.HostStack

		fakeMailer *test.FakeMailer
		origMailer mailer.Mailer
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "user1",
			Email:    "user1@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		pl = &plan.Plan{
			Slug:          "hs1Plan",
			Name:          "hs1Plan",
			MaxContainers: 1,
			Price:         1499,
			StripePlanID:  sql.NullString{"stripe-plan-id", true},
		}
		Expect(pl.Insert(db)).To(BeNil())

		hs, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:        "my-host-1",
				HourlyPrice: 2000,
			},
			P: pl,
			U: u,
		})
		Expect(err).To(BeNil())

		envvars.WebHostname = "nitrous.example"
		origMailer = user.Mailer
		fakeMailer = &test.FakeMailer{}
		subscription.Mailer = fakeMailer
	})

	AfterEach(func() {
		user.Mailer = origMailer
	})

	Describe("SendTrialEndingEmail()", func() {
		var s *subscription.Subscription

		BeforeEach(func() {
			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())
			s = &subscription.Subscription{
				PlanID:           pl.ID,
				BillingProfileID: bp.ID,
				State:            "trialing",
				StripeID:         sql.NullString{"a", true},
				Slug:             "SUB1",
			}
			Expect(test.WrapInTransaction(db, s.Insert)).To(BeNil())
			_, err = db.Exec(`UPDATE subscriptions SET trial_end=$1 WHERE billing_profile_id=$2;`, time.Now().Add(2.5*24*time.Hour), bp.ID)
			Expect(err).To(BeNil())
		})

		It("should send trial ending notice to user", func() {
			subs, err := subscription.FindAllTrialEnding(db, 0, 3*24)
			Expect(err).To(BeNil())

			subscription.SendTrialEndingEmail(db, subs[0], false)

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Tos).To(Equal([]string{`"user1" <user1@example.com>`}))
			Expect(fakeMailer.ReplyTo).To(Equal(envvars.SupportEmail))
			Expect(fakeMailer.Subject).To(Equal("Free Trial Expiring"))

			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("Hello %s!", u.Username)))
			Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("We note that your trial is ending on %v", subs[0].TrialEnd.Time.Format("Monday Jan _2 15:04 2006"))))
			Expect(fakeMailer.Body).To(ContainSubstring(fmt.Sprintf("monthly subscription for our %s plan will begin", pl.Name)))
			Expect(fakeMailer.Body).To(ContainSubstring("We hope you enjoyed using Nitrous"))
			Expect(fakeMailer.Body).To(ContainSubstring("log on to https://www.nitrous.io/app/#/billing to cancel your subscription"))

			Expect(fakeMailer.HTML).To(ContainSubstring("<title>Free Trial Expiring</title>"))
			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("<h3>Hello %s!</h3>", u.Username)))
			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("<p>We note that your trial is ending on <b>%v</b>", subs[0].TrialEnd.Time.Format("Monday Jan _2 15:04 2006"))))
			Expect(fakeMailer.HTML).To(ContainSubstring(fmt.Sprintf("monthly subscription for our %s plan will begin", pl.Name)))
			Expect(fakeMailer.HTML).To(ContainSubstring("<p>We hope you enjoyed using Nitrous"))
			Expect(fakeMailer.HTML).To(ContainSubstring("log on to https://www.nitrous.io/app/#/billing to cancel your subscription"))

		})
	})
})

package idlehosts_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/idlehosts"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	stripe "github.com/stripe/stripe-go"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/idlehosts")
}

var _ = Describe("models/idlehosts", func() {
	var (
		db  *db.Database
		u   *user.User
		hs1 *test.HostStack
		hs2 *test.HostStack
		hs3 *test.HostStack
		hs4 *test.HostStack

		paid, free *plan.Plan
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		u = &user.User{
			Username: "qux",
			Name:     "quxboy",
			Password: "123456",
			Email:    "qux@example.com",
		}
		Expect(test.WrapInTransaction(db, u.Insert)).To(BeNil())

		_, err = u.BillingProfile(db)
		Expect(err).To(BeNil())

		paid = &plan.Plan{Slug: "512mb", Name: "paid", MaxContainers: 1, Price: 500}
		Expect(paid.Insert(db)).To(BeNil())

		free = &plan.Plan{Slug: "64mb", Name: "free", MaxContainers: 1, Price: 0}
		Expect(free.Insert(db)).To(BeNil())

		hs1, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:        "my-host-1",
				Name:        sql.NullString{"Host1", true},
				HourlyPrice: 200,
			},
			U: u,
			P: paid,
			S: &subscription.Subscription{
				Slug:     "sub1",
				StripeID: sql.NullString{"sub_1", true},
			},
		})
		Expect(err).To(BeNil())

		hs2, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:        "my-host-2",
				Name:        sql.NullString{"Host2", true},
				HourlyPrice: 200,
			},
			P: paid,
			U: hs1.U,
			S: &subscription.Subscription{
				Slug:     "sub2",
				StripeID: sql.NullString{"sub_2", true},
			},
		})
		Expect(err).To(BeNil())

		hs3, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:        "my-host-3",
				Name:        sql.NullString{"Host3", true},
				HourlyPrice: 0,
			},
			P: free,
			U: hs1.U,
			S: &subscription.Subscription{
				Slug:     "sub3",
				StripeID: sql.NullString{"sub_3", true},
			},
		})
		Expect(err).To(BeNil())

		hs4, err = test.CreateHostStack(db, &test.HostStack{
			H: &host.Host{
				Slug:        "my-host-4",
				Name:        sql.NullString{"Host4", true},
				HourlyPrice: 0,
			},
			P: free,
			U: hs1.U,
			S: &subscription.Subscription{
				Slug:     "sub4",
				StripeID: sql.NullString{"sub_4", true},
			},
		})
		Expect(err).To(BeNil())

		_, err = db.Exec("UPDATE hosts SET last_running_at = now() - interval '23 days 1 minute' WHERE id IN ($1, $2);", hs1.H.ID, hs3.H.ID)
		Expect(err).To(BeNil())
		_, err = db.Exec("UPDATE hosts SET last_running_at = now() - interval '22 days' WHERE id IN ($1, $2);", hs2.H.ID, hs4.H.ID)
		Expect(err).To(BeNil())
	})

	Describe(".FirstWarningTargets()", func() {
		It("should only include free hosts which have been idle for at least 23 days", func() {
			trgts, err := idlehosts.FirstWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(HaveLen(1))
			Expect(trgts[0].UserName).To(Equal(hs3.U.Name))
			Expect(trgts[0].UserEmail).To(Equal(hs3.U.Email))
			Expect(trgts[0].UserID).To(Equal(hs3.U.ID))
			Expect(trgts[0].UserDeletedAt.Valid).To(BeFalse())
			Expect(trgts[0].HostID).To(Equal(hs3.H.ID))
			Expect(trgts[0].HostSlug).To(Equal(hs3.H.Slug))
			Expect(trgts[0].HostName).To(Equal(hs3.H.Name))
		})

		It("should not include hosts that have been terminated", func() {
			_, err := db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1;", hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.FirstWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})

		It("should not include hosts that were pinged recently", func() {
			_, err := db.Exec("UPDATE hosts SET last_running_at = now() - interval '123 days', last_pinged_at = now() - interval '10 minutes' WHERE id = $1;", hs3.H.ID)
			Expect(err).To(BeNil())
			trgts, err := idlehosts.FirstWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(HaveLen(0))
		})

		It("should not include hosts that have had a notice sent", func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = 1 WHERE id = $1;", hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.FirstWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})

		It("should include hosts that have deleted users", func() {
			_, err := db.Exec("UPDATE users SET deleted_at = now() WHERE id = $1", hs3.U.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.FirstWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(HaveLen(1))
			Expect(trgts[0].HostID).To(Equal(hs3.H.ID))
			Expect(trgts[0].UserDeletedAt.Valid).To(BeTrue())
		})
	})

	Describe(".FinalWarningTargets()", func() {
		BeforeEach(func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1, idle_next_check = now() WHERE id = $2", host.IDLE_NOTICES_FIRST_WARNING, hs3.H.ID)
			Expect(err).To(BeNil())
		})

		It("should include hosts that have received their first notice", func() {
			trgts, err := idlehosts.FinalWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(HaveLen(1))
			Expect(trgts[0].UserName).To(Equal(hs3.U.Name))
			Expect(trgts[0].UserEmail).To(Equal(hs3.U.Email))
			Expect(trgts[0].UserID).To(Equal(hs3.U.ID))
			Expect(trgts[0].UserDeletedAt.Valid).To(BeFalse())
			Expect(trgts[0].HostID).To(Equal(hs3.H.ID))
			Expect(trgts[0].HostSlug).To(Equal(hs3.H.Slug))
			Expect(trgts[0].HostName).To(Equal(hs3.H.Name))
		})

		It("should not include hosts that have not received their first notice", func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1 WHERE id = $2", host.IDLE_NOTICES_NO_WARNING, hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.FinalWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})

		It("should not include hosts that have received their final notice", func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1 WHERE id = $2", host.IDLE_NOTICES_FINAL_WARNING, hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.FinalWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})

		It("should not include hosts that have been terminated", func() {
			_, err := db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1;", hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.FinalWarningTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})
	})

	Describe(".TerminationTargets()", func() {
		BeforeEach(func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1, idle_next_check = now(), last_running_at = now() - interval '30 days 1 minute' WHERE id = $2", host.IDLE_NOTICES_FINAL_WARNING, hs3.H.ID)
			Expect(err).To(BeNil())
		})

		It("should include hosts that have received their final notice", func() {
			trgts, err := idlehosts.TerminationTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(HaveLen(1))
			Expect(trgts[0].UserName).To(Equal(hs3.U.Name))
			Expect(trgts[0].UserEmail).To(Equal(hs3.U.Email))
			Expect(trgts[0].UserID).To(Equal(hs3.U.ID))
			Expect(trgts[0].UserDeletedAt.Valid).To(BeFalse())
			Expect(trgts[0].HostID).To(Equal(hs3.H.ID))
			Expect(trgts[0].HostSlug).To(Equal(hs3.H.Slug))
			Expect(trgts[0].HostName).To(Equal(hs3.H.Name))
		})

		It("should not include hosts that have not received their final notice", func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1 WHERE id = $2", host.IDLE_NOTICES_FIRST_WARNING, hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.TerminationTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})

		It("should not include hosts that have received no notice", func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1 WHERE id = $2", host.IDLE_NOTICES_NO_WARNING, hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.TerminationTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})

		It("should not include hosts that have been terminated", func() {
			_, err := db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1;", hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.TerminationTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})

		It("should not include hosts that have not been idle for 30 days", func() {
			_, err := db.Exec("UPDATE hosts SET last_running_at = now() - interval '29 days 59 minutes' WHERE id = $1;", hs3.H.ID)
			Expect(err).To(BeNil())

			trgts, err := idlehosts.TerminationTargets(db)
			Expect(err).To(BeNil())
			Expect(trgts).To(BeEmpty())
		})
	})

	Describe("SendFirstWarning", func() {
		var (
			target     *idlehosts.Target
			fakeMailer *test.FakeMailer
		)

		BeforeEach(func() {
			target = &idlehosts.Target{
				UserName:     hs1.U.Name,
				UserUsername: hs1.U.Username,
				UserEmail:    hs1.U.Email,
				UserID:       hs1.U.ID,
				HostID:       hs1.H.ID,
				HostSlug:     hs1.H.Slug,
				HostName:     hs1.H.Name,
				SubSlug:      hs1.S.Slug,
			}

			fakeMailer = &test.FakeMailer{}
			idlehosts.Mailer = fakeMailer

			envvars.WebHostname = "nitrous.example"
		})

		It("should send the first warning to the user", func() {
			Expect(target.SendFirstWarning(db)).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(envvars.SupportEmailAddr.String()))
			Expect(fakeMailer.ReplyTo).To(Equal(envvars.SupportEmailAddr.String()))
			Expect(fakeMailer.Tos).To(Equal([]string{`"quxboy" <qux@example.com>`}))
			Expect(fakeMailer.Subject).To(Equal("Nitrous - your workspace 'Host1' will be deleted in 1 week"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hi quxboy,"))
			Expect(fakeMailer.Body).To(ContainSubstring("We noticed you haven't used your free workspace Host1"))
			Expect(fakeMailer.Body).To(ContainSubstring("visit the dashboard at https://nitrous.example/app/"))
			Expect(fakeMailer.Body).To(ContainSubstring("If you do not use your workspace in the next 7 days, it will be deleted."))
			Expect(fakeMailer.Body).To(ContainSubstring(`by upgrading to any of our paid plans at https://nitrous.example/app/#/subscriptions/sub1/upgrade`))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hi quxboy,"))
			Expect(fakeMailer.HTML).To(ContainSubstring("We noticed you haven't used your free workspace <strong>Host1</strong>"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/">visit the dashboard</a>`))
			Expect(fakeMailer.HTML).To(ContainSubstring("If you do not use your workspace in the next 7 days, it will be deleted."))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/#/subscriptions/sub1/upgrade">upgrading to any of our paid plans</a>`))
		})

		It("updates the host to indicate that it has been sent the first warning, and that the next check should be 6 days later", func() {
			Expect(target.SendFirstWarning(db)).To(BeNil())

			h, err := host.FindByIdentifier(db, hs1.H.Slug)
			Expect(err).To(BeNil())
			Expect(h.IdleNoticesSent).To(Equal(host.IDLE_NOTICES_FIRST_WARNING))
			Expect(h.IdleNextCheck.Valid).To(BeTrue())
			nextCheckAfter := 6 * 24 * time.Hour
			Expect(h.IdleNextCheck.Time).To(BeTemporally("~", time.Now().Add(nextCheckAfter), time.Second))
		})

		It("should return an error if the host has an unexpected notice count", func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1 WHERE id = $2;", host.IDLE_NOTICES_FIRST_WARNING, hs1.H.ID)
			Expect(err).To(BeNil())

			err = target.SendFirstWarning(db)
			Expect(err).To(MatchError("Expected idle_notices_sent to equal 0, got 1"))
			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})
	})

	Describe("SendFinalWarning", func() {
		var (
			target     *idlehosts.Target
			fakeMailer *test.FakeMailer
		)

		BeforeEach(func() {
			target = &idlehosts.Target{
				UserName:     hs1.U.Name,
				UserUsername: hs1.U.Username,
				UserEmail:    hs1.U.Email,
				UserID:       hs1.U.ID,
				HostID:       hs1.H.ID,
				HostSlug:     hs1.H.Slug,
				HostName:     hs1.H.Name,
				SubSlug:      hs1.S.Slug,
			}

			fakeMailer = &test.FakeMailer{}
			idlehosts.Mailer = fakeMailer

			envvars.WebHostname = "nitrous.example"

			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1 WHERE id = $2;", host.IDLE_NOTICES_FIRST_WARNING, hs1.H.ID)
			Expect(err).To(BeNil())
		})

		It("should send the first warning to the user", func() {
			Expect(target.SendFinalWarning(db)).To(BeNil())

			Expect(fakeMailer.SendMailCalled).To(BeTrue())
			Expect(fakeMailer.From).To(Equal(envvars.SupportEmailAddr.String()))
			Expect(fakeMailer.ReplyTo).To(Equal(envvars.SupportEmailAddr.String()))
			Expect(fakeMailer.Tos).To(Equal([]string{`"quxboy" <qux@example.com>`}))
			Expect(fakeMailer.Subject).To(Equal("Nitrous - your workspace 'Host1' will be deleted in 24 hours"))
			Expect(fakeMailer.Body).To(ContainSubstring("Hi quxboy,"))
			Expect(fakeMailer.Body).To(ContainSubstring("We noticed you haven't used your free workspace Host1"))
			Expect(fakeMailer.Body).To(ContainSubstring("visit the dashboard at https://nitrous.example/app/"))
			Expect(fakeMailer.Body).To(ContainSubstring("If you do not use your workspace in the next 24 hours, it will be deleted."))
			Expect(fakeMailer.Body).To(ContainSubstring(`by upgrading to any of our paid plans at https://nitrous.example/app/#/subscriptions/sub1/upgrade`))
			Expect(fakeMailer.HTML).To(ContainSubstring("Hi quxboy,"))
			Expect(fakeMailer.HTML).To(ContainSubstring("We noticed you haven't used your free workspace <strong>Host1</strong>"))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/">visit the dashboard</a>`))
			Expect(fakeMailer.HTML).To(ContainSubstring("If you do not use your workspace in the next 24 hours, it will be deleted."))
			Expect(fakeMailer.HTML).To(ContainSubstring(`<a href="https://nitrous.example/app/#/subscriptions/sub1/upgrade">upgrading to any of our paid plans</a>`))
		})

		It("should update the host", func() {
			Expect(target.SendFinalWarning(db)).To(BeNil())

			h, err := host.FindByIdentifier(db, hs1.H.Slug)
			Expect(err).To(BeNil())
			Expect(h.IdleNoticesSent).To(Equal(host.IDLE_NOTICES_FINAL_WARNING))
			Expect(h.IdleNextCheck.Valid).To(BeTrue())
			Expect(h.IdleNextCheck.Time).To(BeTemporally("~", time.Now().Add(24*time.Hour), time.Second))
		})

		It("should return an error if the host has an unexpected notice count", func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1 WHERE id = $2;", host.IDLE_NOTICES_FINAL_WARNING, hs1.H.ID)
			Expect(err).To(BeNil())

			err = target.SendFinalWarning(db)
			Expect(err).To(MatchError("Expected idle_notices_sent to equal 1, got 2"))
			Expect(fakeMailer.SendMailCalled).To(BeFalse())
		})
	})

	Describe("SoftTerminate", func() {
		var (
			target     *idlehosts.Target
			fakeStripe *test.FakeStripe
		)

		BeforeEach(func() {
			target = &idlehosts.Target{
				UserName:     hs1.U.Name,
				UserUsername: hs1.U.Username,
				UserEmail:    hs1.U.Email,
				UserID:       hs1.U.ID,
				HostID:       hs1.H.ID,
				HostSlug:     hs1.H.Slug,
				HostName:     hs1.H.Name,
				SubSlug:      hs1.S.Slug,
			}

			fakeStripe = &test.FakeStripe{}
			stripe.SetBackend(fakeStripe)

			envvars.WebHostname = "nitrous.example"

			bp, err := u.BillingProfile(db)
			Expect(err).To(BeNil())

			bp.StripeID = sql.NullString{"cus_1x", true}
			Expect(bp.Update(db)).To(BeNil())

			_, err = db.Exec("UPDATE plans SET price = 0 WHERE id = $1;", hs1.P.ID)
			Expect(err).To(BeNil())

			_, err = db.Exec("UPDATE hosts SET idle_notices_sent = $1, last_running_at = now() - interval '30 days' WHERE id = $2;", host.IDLE_NOTICES_FINAL_WARNING, hs1.H.ID)
			Expect(err).To(BeNil())
		})

		It("should request stripe cancel the subscription immediately", func() {
			fakeStripe.Errors = []*stripe.Error{nil}
			fakeStripe.Responses = []string{
				`{
					"cancelled": true,
					"id": "sub1"
				}`,
			}

			Expect(target.SoftTerminate(db)).To(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(1))
			r1 := fakeStripe.ReceivedRequests[0]
			Expect(r1["method"]).To(Equal("DELETE"))
			Expect(r1["path"]).To(Equal("/customers/cus_1x/subscriptions/sub_1"))
			Expect(r1["at_period_end"]).To(BeEmpty())
		})

		It("should return an error if the host isn't on a free plan", func() {
			_, err := db.Exec("UPDATE plans SET price = 200 WHERE id = $1;", hs1.P.ID)
			Expect(err).To(BeNil())

			Expect(target.SoftTerminate(db)).To(Equal(sql.ErrNoRows))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(0))
		})

		It("should return an error if the host already has a pending cancel", func() {
			_, err := db.Exec("UPDATE subscriptions SET cancel_pending = TRUE WHERE id = $1;", hs1.S.ID)
			Expect(err).To(BeNil())

			Expect(target.SoftTerminate(db)).To(Equal(sql.ErrNoRows))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(0))
		})

		It("should return an error if the host already has been cancelled", func() {
			_, err := db.Exec("UPDATE subscriptions SET cancelled_at = now(), state = $1 WHERE id = $2;", subscription.StateDeleted, hs1.S.ID)
			Expect(err).To(BeNil())

			Expect(target.SoftTerminate(db)).ToNot(BeNil())

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(0))
		})

		It("should return an error if the host has been terminated", func() {
			_, err := db.Exec("UPDATE hosts SET terminated_at = now() WHERE id = $1;", hs1.H.ID)
			Expect(err).To(BeNil())

			Expect(target.SoftTerminate(db)).To(Equal(sql.ErrNoRows))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(0))
		})

		It("should return an error if the host hasn't been notified twice", func() {
			_, err := db.Exec("UPDATE hosts SET idle_notices_sent = $1 WHERE id = $2;", host.IDLE_NOTICES_FIRST_WARNING, hs1.H.ID)
			Expect(err).To(BeNil())

			Expect(target.SoftTerminate(db)).To(MatchError("Expected idle_notices_sent to equal 2, got 1"))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(0))
		})

		It("should return an error if the host has no last_running_at", func() {
			_, err := db.Exec("UPDATE hosts SET last_running_at = NULL WHERE id = $1;", hs1.H.ID)
			Expect(err).To(BeNil())

			Expect(target.SoftTerminate(db)).To(Equal(idlehosts.ErrLastRunningAtInvalid))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(0))
		})

		It("should return an error if the host has a earlier last_running_at", func() {
			_, err := db.Exec("UPDATE hosts SET last_running_at = now() - interval '13 days' WHERE id = $1;", hs1.H.ID)
			Expect(err).To(BeNil())

			Expect(target.SoftTerminate(db)).To(Equal(idlehosts.ErrHostStartedWithinMaxInactivityDuration))

			Expect(fakeStripe.ReceivedRequests).To(HaveLen(0))
		})
	})
})

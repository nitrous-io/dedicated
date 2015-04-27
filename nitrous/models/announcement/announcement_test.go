package announcement_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/lib/pq"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/announcement")
}

type sentMailStruct struct {
	to, subject, message string
}

type mailSpyStruct struct {
	retErr    error
	sentMails []*sentMailStruct
}

func (s *mailSpyStruct) SendMail(from string, tos []string, ccs []string, bccs []string, replyTo, subject, body, htmltext string) error {
	s.sentMails = append(s.sentMails, &sentMailStruct{
		to:      tos[0],
		subject: subject,
		message: body,
	})

	return s.retErr
}

func createErrorSpy() (func(error), func() error) {
	var caughtError error

	errHandler := func(err error) {
		Expect(err).ToNot(BeNil())
		Expect(caughtError).To(BeNil())
		caughtError = err
	}

	errorCaught := func() error {
		return caughtError
	}

	return errHandler, errorCaught
}

var _ = Describe("models/announcement", func() {
	var (
		db         *ndb.Database
		h1, h2, h3 *host.Host
		u1, u2     *user.User
		expiry     time.Time
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		expiry = time.Now().Add(5 * time.Second)

		u1 = &user.User{
			Username: "foo",
			Email:    "foo@example.org",
		}

		h1 = &host.Host{
			Slug: "elixir-web-1",
		}

		_, err = CreateHostStack(db, &HostStack{
			U: u1,
			H: h1,
		})
		Expect(err).To(BeNil())

		h2 = &host.Host{
			Slug: "elixir-web-2",
		}

		_, err = CreateHostStack(db, &HostStack{
			U: u1,
			H: h2,
		})
		Expect(err).To(BeNil())

		u2 = &user.User{
			Username: "bar",
			Email:    "bar@example.org",
		}

		h3 = &host.Host{
			Slug: "gopher-bar-1",
		}

		_, err = CreateHostStack(db, &HostStack{
			U: u2,
			H: h3,
		})
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Insert global announcement", func() {
		var (
			subjectTemplate = "Global announcement for {{ .Slug }}"
			messageTemplate = "We will be upgrading your host {{ .Slug }} tomorrow."
		)

		BeforeEach(func() {
			insertAnn := &announcement.Announcement{
				SubjectTemplate: subjectTemplate,
				MessageTemplate: messageTemplate,
				ShowToAll:       true,
				ExpiresAt:       pq.NullTime{expiry, true},
			}

			err := announcement.Insert(db, insertAnn)
			Expect(err).To(BeNil())
		})

		It("Should be found in h1", func() {
			anns, err := announcement.FindByHostID(db, h1.ID)
			Expect(err).To(BeNil())

			Expect(len(anns)).To(Equal(1))
			ann := anns[0]

			Expect(ann.ShowToAll).To(BeTrue())
			Expect(ann.HostID).To(Equal(h1.ID))
			Expect(ann.UserID).To(Equal(u1.ID))
			Expect(ann.SubjectTemplate).To(Equal(subjectTemplate))
			Expect(ann.MessageTemplate).To(Equal(messageTemplate))

			subject, err := ann.Subject()
			Expect(err).To(BeNil())
			Expect(subject).To(Equal("Global announcement for elixir-web-1"))

			message, err := ann.Message()
			Expect(err).To(BeNil())
			Expect(message).To(Equal("We will be upgrading your host elixir-web-1 tomorrow."))
		})

		It("Should be found in h2", func() {
			anns, err := announcement.FindByHostID(db, h2.ID)
			Expect(err).To(BeNil())

			Expect(len(anns)).To(Equal(1))
			ann := anns[0]

			Expect(ann.ShowToAll).To(BeTrue())
			Expect(ann.HostID).To(Equal(h2.ID))
			Expect(ann.UserID).To(Equal(u1.ID))
			Expect(ann.SubjectTemplate).To(Equal(subjectTemplate))
			Expect(ann.MessageTemplate).To(Equal(messageTemplate))

			subject, err := ann.Subject()
			Expect(err).To(BeNil())
			Expect(subject).To(Equal("Global announcement for elixir-web-2"))

			message, err := ann.Message()
			Expect(err).To(BeNil())
			Expect(message).To(Equal("We will be upgrading your host elixir-web-2 tomorrow."))
		})

		It("Should be found in h3", func() {
			anns, err := announcement.FindByHostID(db, h3.ID)
			Expect(err).To(BeNil())

			Expect(len(anns)).To(Equal(1))
			ann := anns[0]

			Expect(ann.ShowToAll).To(BeTrue())
			Expect(ann.HostID).To(Equal(h3.ID))
			Expect(ann.UserID).To(Equal(u2.ID))
			Expect(ann.SubjectTemplate).To(Equal(subjectTemplate))
			Expect(ann.MessageTemplate).To(Equal(messageTemplate))

			subject, err := ann.Subject()
			Expect(err).To(BeNil())
			Expect(subject).To(Equal("Global announcement for gopher-bar-1"))

			message, err := ann.Message()
			Expect(err).To(BeNil())
			Expect(message).To(Equal("We will be upgrading your host gopher-bar-1 tomorrow."))
		})

		It("is not found when host is created after announcement", func() {
			_, err := db.Exec(`UPDATE hosts SET created_at= now() + interval '1 day' WHERE id=$1;`, h3.ID)
			Expect(err).To(BeNil())

			anns, err := announcement.FindByHostID(db, h3.ID)
			Expect(err).To(BeNil())
			Expect(len(anns)).To(Equal(0))
		})
	})

	Describe("Insert host announcement", func() {
		var (
			subjectTemplate = "Host announcement for {{ .Slug }}"
			messageTemplate = "We will be upgrading your host {{ .Slug }} next week."
		)

		BeforeEach(func() {
			insertAnn := &announcement.Announcement{
				SubjectTemplate: subjectTemplate,
				MessageTemplate: messageTemplate,
				ShowToAll:       false,
				ExpiresAt:       pq.NullTime{expiry, true},
			}

			insertHosts := []int64{h1.ID, h3.ID}

			err := announcement.InsertForHosts(db, insertAnn, insertHosts)
			Expect(err).To(BeNil())
		})

		It("Should be found in h1", func() {
			anns, err := announcement.FindByHostID(db, h1.ID)
			Expect(err).To(BeNil())

			Expect(len(anns)).To(Equal(1))
			ann := anns[0]

			Expect(ann.ShowToAll).To(BeFalse())
			Expect(ann.HostID).To(Equal(h1.ID))
			Expect(ann.UserID).To(Equal(u1.ID))
			Expect(ann.SubjectTemplate).To(Equal(subjectTemplate))
			Expect(ann.MessageTemplate).To(Equal(messageTemplate))

			subject, err := ann.Subject()
			Expect(err).To(BeNil())
			Expect(subject).To(Equal("Host announcement for elixir-web-1"))

			message, err := ann.Message()
			Expect(err).To(BeNil())
			Expect(message).To(Equal("We will be upgrading your host elixir-web-1 next week."))
		})

		It("Should not found in h2", func() {
			anns, err := announcement.FindByHostID(db, h2.ID)
			Expect(err).To(BeNil())

			Expect(len(anns)).To(Equal(0))
		})

		It("Should be found in h3", func() {
			anns, err := announcement.FindByHostID(db, h3.ID)
			Expect(err).To(BeNil())

			Expect(len(anns)).To(Equal(1))
			ann := anns[0]

			Expect(ann.ShowToAll).To(BeFalse())
			Expect(ann.HostID).To(Equal(h3.ID))
			Expect(ann.UserID).To(Equal(u2.ID))
			Expect(ann.SubjectTemplate).To(Equal(subjectTemplate))
			Expect(ann.MessageTemplate).To(Equal(messageTemplate))

			subject, err := ann.Subject()
			Expect(err).To(BeNil())
			Expect(subject).To(Equal("Host announcement for gopher-bar-1"))

			message, err := ann.Message()
			Expect(err).To(BeNil())
			Expect(message).To(Equal("We will be upgrading your host gopher-bar-1 next week."))
		})

		It("is not found when host is created after announcement", func() {
			_, err := db.Exec(`UPDATE hosts SET created_at= now() + interval '1 day' WHERE id=$1;`, h3.ID)
			Expect(err).To(BeNil())

			anns, err := announcement.FindByHostID(db, h3.ID)
			Expect(err).To(BeNil())
			Expect(len(anns)).To(Equal(0))
		})
	})

	Describe("Announcement emails", func() {
		var (
			mailSpy    *mailSpyStruct
			origMailer mailer.Mailer
		)

		BeforeEach(func() {
			origMailer = user.Mailer
			mailSpy = &mailSpyStruct{}
			user.Mailer = mailSpy
		})

		AfterEach(func() {
			user.Mailer = origMailer
		})

		// Assert all mails should eventually be sent asynchronously in goroutine
		assertSentMails := func(mailSpy *mailSpyStruct, expectedMails []*sentMailStruct, errorCaught func() error) {
			Eventually(func() error {
				err := errorCaught()
				if err != nil {
					return err
				}

				if len(expectedMails) == len(mailSpy.sentMails) {
					return nil
				}
				return fmt.Errorf("Expected to receive %d emails, only received %d emails", len(expectedMails), len(mailSpy.sentMails))
			}, 100*time.Millisecond).Should(BeNil())
			Expect(mailSpy.sentMails).To(ConsistOf(expectedMails))
		}

		Context("Global Announcement", func() {
			var (
				subjectTemplate = "Global announcement for {{ .Slug }}"
				messageTemplate = "We will be upgrading your host {{ .Slug }} tomorrow."

				ann = &announcement.Announcement{
					SubjectTemplate: subjectTemplate,
					MessageTemplate: messageTemplate,
					ShowToAll:       true,
					ExpiresAt:       pq.NullTime{expiry, true},
				}

				expectedSentMails []*sentMailStruct
			)

			BeforeEach(func() {
				expectedSentMails = []*sentMailStruct{
					{
						to:      "<foo@example.org>",
						subject: "Global announcement for elixir-web-1",
						message: "Hello foo,\n\nWe will be upgrading your host elixir-web-1 tomorrow.\n\nThanks,\nThe Nitrous Team",
					},
					{
						to:      "<foo@example.org>",
						subject: "Global announcement for elixir-web-2",
						message: "Hello foo,\n\nWe will be upgrading your host elixir-web-2 tomorrow.\n\nThanks,\nThe Nitrous Team",
					},
					{
						to:      "<bar@example.org>",
						subject: "Global announcement for gopher-bar-1",
						message: "Hello bar,\n\nWe will be upgrading your host gopher-bar-1 tomorrow.\n\nThanks,\nThe Nitrous Team",
					},
				}
			})

			It("Global Announcement should send to all", func() {
				errHandler, errorCaught := createErrorSpy()

				err := announcement.SendAndSaveGlobalAnnouncement(db, ann, errHandler)
				Expect(err).To(BeNil())

				assertSentMails(mailSpy, expectedSentMails, errorCaught)
			})

			It("Global announcement should not sent to terminated host", func() {
				_, err := db.Exec(`UPDATE hosts SET terminated_at=now() WHERE id=$1;`, h2.ID)
				Expect(err).To(BeNil())

				errHandler, errorCaught := createErrorSpy()

				err = announcement.SendAndSaveGlobalAnnouncement(db, ann, errHandler)
				Expect(err).To(BeNil())

				newExpectedMails := []*sentMailStruct{
					expectedSentMails[0],
					expectedSentMails[2],
				}

				assertSentMails(mailSpy, newExpectedMails, errorCaught)
			})

			It("Global announcement should not sent to preprovisioned host", func() {
				_, err := db.Exec(`UPDATE hosts SET preprovisioned=true WHERE id=$1;`, h3.ID)
				Expect(err).To(BeNil())

				errHandler, errorCaught := createErrorSpy()

				err = announcement.SendAndSaveGlobalAnnouncement(db, ann, errHandler)
				Expect(err).To(BeNil())

				newExpectedMails := []*sentMailStruct{
					expectedSentMails[0],
					expectedSentMails[1],
				}

				assertSentMails(mailSpy, newExpectedMails, errorCaught)
			})
		})

		Context("Host Announcement", func() {
			var (
				subjectTemplate = "Host announcement for {{ .Slug }}"
				messageTemplate = "We will be upgrading your host {{ .Slug }} next week."

				expectedSentMails = []*sentMailStruct{
					{
						to:      "<foo@example.org>",
						subject: "Host announcement for elixir-web-2",
						message: "Hello foo,\n\nWe will be upgrading your host elixir-web-2 next week.\n\nThanks,\nThe Nitrous Team",
					},
					{
						to:      "<bar@example.org>",
						subject: "Host announcement for gopher-bar-1",
						message: "Hello bar,\n\nWe will be upgrading your host gopher-bar-1 next week.\n\nThanks,\nThe Nitrous Team",
					},
				}
			)

			It("Host Announcement should send to specified slugs", func() {
				ann := &announcement.Announcement{
					SubjectTemplate: subjectTemplate,
					MessageTemplate: messageTemplate,
					ShowToAll:       false,
					ExpiresAt:       pq.NullTime{expiry, true},
				}

				errHandler, errorCaught := createErrorSpy()

				slugs := []string{
					"elixir-web-2",
					"gopher-bar-1",
				}

				hostAnns, err := announcement.SendAndSaveForHostSlugs(db, ann, slugs, errHandler)
				Expect(err).To(BeNil())

				type annSlice []*announcement.Announcement
				Expect(annSlice{hostAnns[0].Announcement, hostAnns[1].Announcement}).To(ConsistOf(annSlice{ann, ann}))

				Expect([]string{hostAnns[0].Slug, hostAnns[1].Slug}).To(ConsistOf([]string{slugs[0], slugs[1]}))

				assertSentMails(mailSpy, expectedSentMails, errorCaught)
			})
		})
	})
})

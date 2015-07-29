package host_test

import (
	"database/sql"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/lib/pq/hstore"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/host")
}

var _ = Describe("models/host", func() {
	var (
		db *ndb.Database
		hs *test.HostStack
		s  *subscription.Subscription
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err = test.CreateHostStack(db, &test.HostStack{
			PT: &providertemplate.ProviderTemplate{
				Name:     "default-1",
				Provider: "amazon",
				Config: hstore.Hstore{
					Map: map[string]sql.NullString{
						"disable_eip": sql.NullString{"true", true},
					},
				},
			},
		})
		Expect(err).To(BeNil())

		// Insert subscriptions
		bp, err := hs.U.BillingProfile(db)
		Expect(err).To(BeNil())

		s = &subscription.Subscription{
			PlanID:           hs.P.ID,
			BillingProfileID: bp.ID,
			State:            "active",
			StripeID:         sql.NullString{"sub_ABC", true},
			Slug:             "Sub1",
		}
		err = test.WrapInTransaction(db, s.Insert)
		Expect(err).To(BeNil())

		_, err = db.Exec("UPDATE hosts SET subscription_id = $1 WHERE id = $2;", s.ID, hs.H.ID)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("String()", func() {
		It("redacts host secret", func() {
			h := hs.H
			s := fmt.Sprintf("%+v", h)
			Expect(s).NotTo(ContainSubstring(h.Secret))
			Expect(s).To(ContainSubstring("Secret:(REDACTED)"))

			s = fmt.Sprintf("%v", h)
			Expect(s).NotTo(ContainSubstring(h.Secret))
			Expect(s).To(ContainSubstring("Secret:(REDACTED)"))
		})
	})

	Describe("Touch()", func() {
		It("updates last_pinged_at", func() {
			h := hs.H
			Expect(time.Now().Sub(h.LastPingedAt.Time)).NotTo(BeNumerically("<=", 1*time.Second))

			err := h.Touch(db)
			Expect(err).To(BeNil())

			h, err = host.FindByIdentifier(db, strconv.FormatInt(h.ID, 10))
			Expect(err).To(BeNil())
			Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
		})

		Context("when some error occurs", func() {
			It("returns the error", func() {
				db.Close()
				err := hs.H.Touch(db)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindAllByProvider()", func() {
		var (
			fv = "1.0.69-deadbeef-88"
			dv = "1.4.1-nitrous"
		)

		It("returns all (unterminated) hosts of the given provider", func() {
			// Terminate the host created with the shared host stack so we don't
			// have to account for it.
			err := hs.H.Delete(db)
			Expect(err).To(BeNil())

			u1 := &user.User{
				Username: "arunthampi",
				Email:    "arunthampi@gmail.com",
				Password: "teh interview is osom",
			}
			err = test.WrapInTransaction(db, u1.Insert)

			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version)
				VALUES ($1, $2, $3, $4, $5, $6, $7);`,
				u1.ID, "aruns-second-host", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version, terminated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8);`, // Terminated host.
				u1.ID, "aruns-first-host", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv, time.Now())
			Expect(err).To(BeNil())

			// Create some pre-provisioned hosts.
			u2 := &user.User{
				Username: "pre-provisioner",
				Email:    "pp@example.com",
				Password: "pp1234",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned, state)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
				u2.ID, "preprovisioned-host-1", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true, host.StateRunning)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id,preprovisioned, terminated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`, // Terminated host.
				u2.ID, "preprovisioned-host-2", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true, time.Now())
			Expect(err).To(BeNil())

			// Create a host belonging to a different provider.
			_, err = test.CreateHostStack(db, &test.HostStack{
				H:  &host.Host{Slug: "other-1"},
				PT: &providertemplate.ProviderTemplate{Provider: "some-other-provider"},
			})
			Expect(err).To(BeNil())

			hosts, err := host.FindAllByProvider(db, hs.PT.Provider)
			Expect(err).To(BeNil())

			Expect(hosts).To(HaveLen(2))
			Expect(hosts[0].Slug).To(Equal("aruns-second-host"))
			Expect(hosts[1].Slug).To(Equal("preprovisioned-host-1"))
		})
	})

	Describe("FindAllByUser()", func() {
		var (
			u1 *user.User
			u2 *user.User
		)

		BeforeEach(func() {
			var err error
			fv := "1.0.69-deadbeef-88"
			dv := "1.4.1-nitrous"

			u1 = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u1.Insert)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			p := &plan.Plan{
				Slug:          "plan-a",
				Name:          "cozy-workplace",
				MaxContainers: 16,
				Storage:       "1024 GB",
				Price:         1024,
				Memory:        "68 GB",
			}
			p.Insert(db)

			// Insert subscriptions
			bp, err := u1.BillingProfile(db)
			Expect(err).To(BeNil())

			s := &subscription.Subscription{
				PlanID:           p.ID,
				BillingProfileID: bp.ID,
				State:            "active",
				StripeID:         sql.NullString{"1", true},
				Slug:             "subslug",
			}
			err = test.WrapInTransaction(db, s.Insert)
			Expect(err).To(BeNil())

			// insert a valid running host
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version, subscription_id)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8);`,
				u1.ID, "my-host-1", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv, s.ID)
			Expect(err).To(BeNil())

			// insert a terminated host
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version, terminated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, now());`,
				u1.ID, "my-host-2", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv)
			Expect(err).To(BeNil())

			// insert a orphaned host
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version, terminated_at, state)
				VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8);`,
				u1.ID, "my-host-3", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv, "orphan")
			Expect(err).To(BeNil())

			// insert a migrating host
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version, terminated_at, state)
				VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8);`,
				u1.ID, "my-host-4", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv, "migrating")
			Expect(err).To(BeNil())
		})

		Context("when the given user has host records", func() {
			It("returns the host records", func() {
				hosts, err := host.FindAllByUser(db, u1.ID)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(1))

				Expect(hosts[0].ID).NotTo(BeZero())
				Expect(hosts[0].UserID).To(Equal(u1.ID))
				Expect(hosts[0].Slug).To(Equal("my-host-1"))
				Expect(hosts[0].SubscriptionSlug.String).To(Equal("subslug"))
			})
		})

		Context("when the given user has host records but deleted subscriptions", func() {
			BeforeEach(func() {
				s, err := subscription.FindBySlug(db, "subslug", u1.ID)
				Expect(err).To(BeNil())
				s.State = subscription.StateDeleted
				err = s.Update(db)
				Expect(err).To(BeNil())
			})

			It("does not return those records", func() {
				hosts, err := host.FindAllByUser(db, u1.ID)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(0))
			})
		})

		Context("when the given user does not have any host records", func() {
			It("returns an empty slice", func() {
				hosts, err := host.FindAllByUser(db, u2.ID)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(0))
			})
		})
	})

	Describe("FindAllWithTerminated()", func() {
		var (
			u1 *user.User
			u2 *user.User
		)

		BeforeEach(func() {
			var err error
			fv := "1.0.69-deadbeef-88"
			dv := "1.4.1-nitrous"

			u1 = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u1.Insert)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version)
				VALUES ($1, $2, $3, $4, $5, $6, $7);`,
				u1.ID, "my-host-1", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version, terminated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, now());`,
				u1.ID, "my-host-2", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv)
			Expect(err).To(BeNil())
		})

		Context("when the given user has host records", func() {
			It("returns the host records", func() {
				hosts, err := host.FindAllWithTerminated(db, u1.ID)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(2))

				Expect(hosts[0].ID).NotTo(BeZero())
				Expect(hosts[0].UserID).To(Equal(u1.ID))
				Expect(hosts[0].Slug).To(Equal("my-host-1"))

				Expect(hosts[1].ID).NotTo(BeZero())
				Expect(hosts[1].UserID).To(Equal(u1.ID))
				Expect(hosts[1].Slug).To(Equal("my-host-2"))
			})
		})

		Context("when the given user does not have any host records", func() {
			It("returns an empty slice", func() {
				hosts, err := host.FindAllWithTerminated(db, u2.ID)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(0))
			})
		})
	})

	Describe("CountPreprovisioned()", func() {
		var hs2 *test.HostStack

		BeforeEach(func() {
			var err error
			fv := "1.0.69-deadbeef-88"
			dv := "1.4.1-nitrous"
			u := &user.User{
				Username: "pre-provisioner",
				Email:    "pp@example.com",
				Password: "pp1234",
			}
			err = test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO hosts
			(user_id, slug, region_id, plan_id, provider_template_id, freighterd_version, docker_version, preprovisioned)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8);`,
				u.ID, "preprovisioned-host-1", hs.R.ID, hs.P.ID, hs.PT.ID, fv, dv, true)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
			(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned, state)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
				u.ID, "preprovisioned-host-2", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true, host.StateRunning)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
			(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id,preprovisioned, terminated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
				u.ID, "preprovisioned-host-3", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true, time.Now())
			Expect(err).To(BeNil())

			hs2, err = test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8);`,
				u.ID, "preprovisioned-host-4", hs2.R.ID, hs2.P.ID, fv, dv, hs2.PT.ID, true)
			Expect(err).To(BeNil())
		})

		It("returns the count of non-terminated pre-provisioned hosts in the given region and plan", func() {
			count, err := host.CountPreprovisioned(db, hs.R, hs.P)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("returns 0 if there are no pre-provisioned hosts in the given region and plan", func() {
			count, err := host.CountPreprovisioned(db, hs.R, hs2.P)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
		})
	})

	Describe("CountPreprovisionedOrphans()", func() {
		var hs2 *test.HostStack
		fv := "1.0.69-deadbeef-88"
		dv := "1.4.1-nitrous"

		BeforeEach(func() {
			var err error
			u := &user.User{
				Username: "pre-provisioner",
				Email:    "pp@example.com",
				Password: "pp1234",
			}
			err = test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned, state)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
				u.ID, "preprovisioned-host-1", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true, host.StateRunning)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned, state)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
				u.ID, "preprovisioned-host-2", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true, host.StateRunning)
			Expect(err).To(BeNil())

			hs2, err = test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned, state)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
				u.ID, "preprovisioned-host-3", hs2.R.ID, hs2.P.ID, fv, dv, hs2.PT.ID, true, host.StateOrphan)
			Expect(err).To(BeNil())
		})

		It("returns the count of orphan pre-provisioned hosts in the given region", func() {
			count, err := host.CountPreprovisionedOrphans(db, hs.R)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))

			count, err = host.CountPreprovisionedOrphans(db, hs2.R)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})

		It("returns 0 if there are no orphan pre-provisioned hosts in the given region", func() {
			count, err := host.CountPreprovisionedOrphans(db, hs.R)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(0))
		})
	})

	Describe("Find()", func() {
		Context("when given param is an ID", func() {
			It("returns the matching record that belongs to the user", func() {
				h, err := host.Find(db, hs.U, strconv.FormatInt(hs.H.ID, 10))
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.UserID).To(Equal(hs.U.ID))
				Expect(h.SubscriptionSlug.String).To(Equal(s.Slug))
				Expect(h.Slug).To(Equal(hs.H.Slug))
				Expect(h.Provider).To(Equal(hs.PT.Provider))
				Expect(h.ProviderTemplate.Config.Map["disable_eip"]).To(Equal(sql.NullString{"true", true}))
			})

			It("returns an error when the matching record does not belong to the user", func() {
				u := &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				h, err := host.Find(db, u, strconv.FormatInt(hs.H.ID, 10))
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})

			It("returns an error when there's no matching record", func() {
				h, err := host.Find(db, hs.U, "10001")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})
		})

		Context("when given param is a slug", func() {
			It("returns the matching host record that belongs to the user", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.SubscriptionSlug.String).To(Equal(s.Slug))
				Expect(h.UserID).To(Equal(hs.U.ID))
				Expect(h.Slug).To(Equal(hs.H.Slug))
				Expect(h.Provider).To(Equal(hs.PT.Provider))
				Expect(h.ProviderTemplate.Config.Map["disable_eip"]).To(Equal(sql.NullString{"true", true}))
			})

			It("returns an error when matching record does not belong to the user", func() {
				u := &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := test.WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				h, err := host.Find(db, u, hs.H.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})

			It("returns an error when there's no matching record'", func() {
				h, err := host.Find(db, hs.U, "vivabrazil")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(h).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindShared()", func() {
		var (
			recv *user.User
			c    *container.Container
		)

		BeforeEach(func() {
			recv = &user.User{
				Username: "shared",
				Email:    "shared@example.com",
				Password: "123456",
			}
			err := test.WrapInTransaction(db, recv.Insert)
			Expect(err).To(BeNil())

			c = &container.Container{
				CID:    "thingo3",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "coffee-test-container",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`, c.ID, recv.ID)
			Expect(err).To(BeNil())
		})

		Context("when given param is an ID", func() {
			It("returns the matching record that is shared via the container", func() {
				h, err := host.FindShared(db, recv, strconv.FormatInt(hs.H.ID, 10))
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.UserID).To(Equal(hs.U.ID))
				Expect(h.Slug).To(Equal(hs.H.Slug))
				Expect(h.Provider).To(Equal(hs.PT.Provider))
			})

			It("returns an error when the matching record is not shared with the user", func() {
				_, err := db.Exec(`DELETE FROM container_collabs WHERE user_id = $1;`, recv.ID)
				Expect(err).To(BeNil())

				h, err := host.FindShared(db, recv, strconv.FormatInt(hs.H.ID, 10))
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})

			It("returns an error when there's no matching record", func() {
				h, err := host.FindShared(db, recv, "10001")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})
		})

		Context("when given param is a slug", func() {
			It("returns the matching host record that is shared via the container", func() {
				h, err := host.FindShared(db, recv, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.UserID).To(Equal(hs.U.ID))
				Expect(h.Slug).To(Equal(hs.H.Slug))
				Expect(h.Provider).To(Equal(hs.PT.Provider))
				Expect(h.ProviderTemplate.Config.Map["disable_eip"]).To(Equal(sql.NullString{"true", true}))
			})

			It("returns an error when matching record is not shared with the user", func() {
				_, err := db.Exec(`DELETE FROM container_collabs WHERE user_id = $1;`, recv.ID)
				Expect(err).To(BeNil())

				h, err := host.FindShared(db, recv, hs.H.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})

			It("returns an error when there's no matching record'", func() {
				h, err := host.FindShared(db, recv, "vivabrazil")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				h, err := host.FindShared(db, recv, hs.H.Slug)
				Expect(h).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindByIdentifier()", func() {
		Context("when given identifier is an ID", func() {
			It("returns the host with the given ID", func() {
				h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
				Expect(err).To(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.ProviderTemplate.Config.Map["disable_eip"]).To(Equal(sql.NullString{"true", true}))
			})

			It("returns pre-provisioned host with the given ID", func() {
				hs.H.Preprovisioned = true
				err := hs.H.Update(db)
				Expect(err).To(BeNil())

				h, err := host.FindByIdentifier(db, strconv.FormatInt(hs.H.ID, 10))
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.Preprovisioned).To(BeTrue())
			})

			It("returns an error when there's no matching record", func() {
				h, err := host.FindByIdentifier(db, "10001")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})
		})

		Context("when given identifier is a slug", func() {
			It("returns the host with the given slug", func() {
				h, err := host.FindByIdentifier(db, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.Slug).To(Equal(hs.H.Slug))
				Expect(h.ProviderTemplate.Config.Map["disable_eip"]).To(Equal(sql.NullString{"true", true}))
			})

			It("returns pre-provisioned host with the given slug", func() {
				hs.H.Preprovisioned = true
				err := hs.H.Update(db)
				Expect(err).To(BeNil())

				h, err := host.FindByIdentifier(db, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.Slug).To(Equal(hs.H.Slug))
				Expect(h.Preprovisioned).To(BeTrue())
			})

			It("returns an error when there's no matching record'", func() {
				h, err := host.FindByIdentifier(db, "vivabrazil")
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(h).To(BeNil())
			})
		})
	})

	Describe("FindPreprovisioned()", func() {
		BeforeEach(func() {
			var err error
			fv := "1.0.69-deadbeef-88"
			dv := "1.4.1-nitrous"
			u := &user.User{
				Username: "pre-provisioner",
				Email:    "pp@example.com",
				Password: "pp1234",
			}
			err = test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8);`,
				u.ID, "preprovisioned-host-1", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned, state)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
				u.ID, "preprovisioned-host-2", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true, host.StateRunning)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, freighterd_version, docker_version, provider_template_id, preprovisioned, terminated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);`,
				u.ID, "preprovisioned-host-3", hs.R.ID, hs.P.ID, fv, dv, hs.PT.ID, true, time.Now())
			Expect(err).To(BeNil())
		})

		It("returns a non-terminated pre-provisioned host in the given region and plan", func() {
			h, err := host.FindPreprovisioned(db, hs.R, hs.P)
			Expect(err).To(BeNil())
			Expect(h.RegionID).To(Equal(hs.R.ID))
			Expect(h.PlanID).To(Equal(hs.P.ID))
			Expect(h.ProviderTemplateID).To(Equal(hs.PT.ID))
			Expect(h.Slug).To(Equal("preprovisioned-host-2"))
		})

		It("does not find the host if there are no pre-provisioned hosts in the given region and plan", func() {
			hs2, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())

			h, err := host.FindPreprovisioned(db, hs2.R, hs2.P)
			Expect(err).To(MatchError(sql.ErrNoRows))
			Expect(h).To(BeNil())
		})
	})

	Describe("Insert()", func() {
		var (
			h *host.Host
			t time.Time
			u *user.User
		)

		BeforeEach(func() {
			var err error
			fv := "1.0.69-deadbeef-88"
			dv := "1.4.1-nitrous"

			t = time.Now()

			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			h = &host.Host{
				UserID:             u.ID,
				Slug:               "my-host-1",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				HourlyPrice:        int64(634),
				Free:               true,
				LastPingedAt:       pq.NullTime{t, true},
				AutoShutdown:       true,
				FreighterdVersion:  fv,
				DockerVersion:      dv,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the host in the database", func() {
			hosts, err := host.FindAllByUser(db, u.ID)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			Expect(hosts[0].ID).NotTo(BeZero())
			Expect(hosts[0].UserID).To(Equal(u.ID))
			Expect(hosts[0].Slug).To(Equal("my-host-1"))
			Expect(hosts[0].HourlyPrice).To(Equal(int64(634)))
			Expect(hosts[0].Free).To(BeTrue())
			Expect(hosts[0].FreighterdVersion).To(Equal("1.0.69-deadbeef-88"))
			Expect(hosts[0].DockerVersion).To(Equal("1.4.1-nitrous"))
			Expect(hosts[0].CreatedAt).NotTo(BeNil())
			Expect(hosts[0].UpdatedAt).NotTo(BeNil())
			Expect(hosts[0].LastPingedAt.Time.Unix()).To(Equal(t.Unix()))
			Expect(hosts[0].AutoShutdown).To(BeTrue())
			Expect(hosts[0].FreighterdVersion).To(Equal("1.0.69-deadbeef-88"))
			Expect(hosts[0].DockerVersion).To(Equal("1.4.1-nitrous"))
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(h.ID).NotTo(BeZero())
			Expect(h.CreatedAt).NotTo(BeNil())
			Expect(h.UpdatedAt).NotTo(BeNil())
		})

		It("sets state to a default of 'born'", func() {
			Expect(h.State).To(Equal("born"))
		})

		It("generates a new secret", func() {
			Expect(h.Secret).NotTo(BeEmpty())
			Expect(h.Secret).To(HaveLen(255))

			h2 := &host.Host{
				UserID:             u.ID,
				Slug:               "my-host-2",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			err := h2.Insert(db)
			Expect(err).To(BeNil())
			Expect(h2.Secret).NotTo(BeEmpty())
			Expect(h2.Secret).To(HaveLen(255))
			Expect(h2.Secret).NotTo(Equal(h.Secret))
		})
	})

	Describe("Update()", func() {
		var (
			h    *host.Host
			slug = "foo"
		)

		BeforeEach(func() {
			var err error
			h = &host.Host{
				Slug:               slug,
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              host.StateRunning,
				HourlyPrice:        100,
				Free:               false,
				TerminatedAt:       nil,
				Preprovisioned:     false,
				LastPingedAt:       pq.NullTime{Valid: false},
				AutoShutdown:       false,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the host in the database", func() {
			t := time.Now()

			h.Slug = "new-slug"
			h.Name = sql.NullString{"foo host", true}
			h.State = host.StateRestarting
			h.IPAddress = sql.NullString{"1.2.3.4", true}
			h.ProviderIdentifier = sql.NullString{"digitalocean", true}
			h.MaxContainers = sql.NullInt64{2, true}
			h.FreighterdPort = sql.NullInt64{60000, true}
			h.Preprovisioned = true
			h.FreighterdVersion = "0.1.0"
			h.DockerVersion = "1.5.0"
			h.HourlyPrice = 0
			h.Free = true
			h.LastPingedAt = pq.NullTime{t, true}
			h.AutoShutdown = true
			err := h.Update(db)
			Expect(err).To(BeNil())

			h2 := &host.Host{}
			err = db.Get(h2, `SELECT * FROM hosts WHERE id = $1`, h.ID)
			Expect(err).To(BeNil())
			Expect(h2).NotTo(BeNil())

			// Updated fields.
			Expect(h2.Slug).To(Equal("new-slug"))
			Expect(h2.Name.String).To(Equal("foo host"))
			Expect(h2.State).To(Equal(host.StateRestarting))
			Expect(h2.IPAddress.String).To(Equal("1.2.3.4"))
			Expect(h2.ProviderIdentifier.String).To(Equal("digitalocean"))
			Expect(h2.MaxContainers.Int64).To(Equal(int64(2)))
			Expect(h2.FreighterdPort.Int64).To(Equal(int64(60000)))
			Expect(h2.Preprovisioned).To(BeTrue())
			Expect(h2.FreighterdVersion).To(Equal("0.1.0"))
			Expect(h2.DockerVersion).To(Equal("1.5.0"))
			Expect(h2.LastPingedAt.Valid).To(BeTrue())
			Expect(h2.LastPingedAt.Time.Unix()).To(Equal(t.Unix()))
			Expect(h2.AutoShutdown).To(BeTrue())

			// Unchanged fields.
			Expect(h2.UserID).To(Equal(hs.U.ID))
			Expect(h2.RegionID).To(Equal(hs.R.ID))
			Expect(h2.PlanID).To(Equal(hs.P.ID))
			Expect(h2.ProviderTemplateID).To(Equal(hs.PT.ID))
			Expect(h2.HourlyPrice).To(Equal(int64(100)))
			Expect(h2.Free).To(Equal(false))
		})

		It("fills up the fields in original struct", func() {
			h.Name = sql.NullString{"foo host", true}
			err := h.Update(db)
			Expect(err).To(BeNil())

			Expect(h.CreatedAt).NotTo(BeNil())
			Expect(h.UpdatedAt).NotTo(BeNil())
		})

		It("updates UpdatedAt", func() {
			origUpdatedAt := *h.UpdatedAt
			time.Sleep(1 * time.Millisecond)

			h.Name = sql.NullString{"foo host", true}
			err := h.Update(db)
			Expect(err).To(BeNil())

			Expect(h.UpdatedAt.UnixNano()).NotTo(Equal(origUpdatedAt.UnixNano()))
			Expect(time.Now().Sub(*h.UpdatedAt)).To(BeNumerically("<=", 1*time.Second))
		})
	})

	Describe("UpdateState()", func() {
		BeforeEach(func() {
			var err error
			h := &host.Host{
				Slug:               "foo",
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              host.StateRunning,
				HourlyPrice:        100,
				Free:               false,
				TerminatedAt:       nil,
				Preprovisioned:     false,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("should not update if original state does not match latest state in database", func() {
			h1, err := host.FindByIdentifier(db, "foo")
			Expect(err).To(BeNil())

			h2, err := host.FindByIdentifier(db, "foo")
			Expect(err).To(BeNil())

			Expect(h1.State).To(Equal(host.StateRunning))

			err = h1.UpdateState(db, host.StateStopping)
			Expect(err).To(BeNil())
			Expect(h1.State).To(Equal(host.StateStopping))

			Expect(h2.State).To(Equal(host.StateRunning))
			err = h2.UpdateState(db, host.StateStopping)
			Expect(err).To(Equal(host.ErrUpdateFailed))
			Expect(h2.State).To(Equal(host.StateRunning))

			h3, err := host.FindByIdentifier(db, "foo")
			Expect(err).To(BeNil())
			Expect(h3.State).To(Equal(host.StateStopping))
		})
	})

	Describe("UpdateColumn()", func() {
		var (
			h  *host.Host
			h2 *host.Host

			updatedAt time.Time

			slug = "foo"
		)

		BeforeEach(func() {
			var err error
			h = &host.Host{
				Slug:               slug,
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
			updatedAt = *h.UpdatedAt

			time.Sleep(1 * time.Millisecond)
			err = h.UpdateColumn(db, "slug", "bar")
			Expect(err).To(BeNil())

			h2 = &host.Host{}
			err = db.Get(h2, `SELECT * FROM hosts WHERE id = $1`, h.ID)
			Expect(err).To(BeNil())
			Expect(h2).NotTo(BeNil())
		})

		It("updates a single column", func() {
			Expect(h2.Slug).To(Equal("bar"))
		})

		It("also updates updated_at", func() {
			Expect(h2.UpdatedAt.UnixNano()).NotTo(Equal(updatedAt.UnixNano()))
			Expect(time.Now().Sub(*h2.UpdatedAt)).To(BeNumerically("<=", 1*time.Second))
		})
	})

	Describe("UpdateWithPrice()", func() {
		var (
			h    *host.Host
			slug = "foo"
		)

		BeforeEach(func() {
			var err error
			h = &host.Host{
				Slug:               slug,
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              host.StateRunning,
				HourlyPrice:        100,
				Free:               false,
				TerminatedAt:       nil,
				Preprovisioned:     false,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the host in the database", func() {
			h.Slug = "new-slug"
			h.Name = sql.NullString{"foo host", true}
			h.State = host.StateRestarting
			h.IPAddress = sql.NullString{"1.2.3.4", true}
			h.ProviderIdentifier = sql.NullString{"digitalocean", true}
			h.MaxContainers = sql.NullInt64{2, true}
			h.FreighterdPort = sql.NullInt64{60000, true}
			h.Preprovisioned = true
			h.FreighterdVersion = "0.1.0"
			h.DockerVersion = "1.5.0"
			h.HourlyPrice = 0
			h.Free = true
			err := h.UpdateWithPrice(db)
			Expect(err).To(BeNil())

			h2 := &host.Host{}
			err = db.Get(h2, `SELECT * FROM hosts WHERE id = $1`, h.ID)
			Expect(err).To(BeNil())
			Expect(h2).NotTo(BeNil())

			// Updated fields.
			Expect(h2.Slug).To(Equal("new-slug"))
			Expect(h2.Name.String).To(Equal("foo host"))
			Expect(h2.State).To(Equal(host.StateRestarting))
			Expect(h2.IPAddress.String).To(Equal("1.2.3.4"))
			Expect(h2.ProviderIdentifier.String).To(Equal("digitalocean"))
			Expect(h2.MaxContainers.Int64).To(Equal(int64(2)))
			Expect(h2.FreighterdPort.Int64).To(Equal(int64(60000)))
			Expect(h2.Preprovisioned).To(BeTrue())
			Expect(h2.FreighterdVersion).To(Equal("0.1.0"))
			Expect(h2.DockerVersion).To(Equal("1.5.0"))
			Expect(h2.HourlyPrice).To(Equal(int64(0)))
			Expect(h2.Free).To(Equal(true))

			// Unchanged fields.
			Expect(h2.UserID).To(Equal(hs.U.ID))
			Expect(h2.RegionID).To(Equal(hs.R.ID))
			Expect(h2.PlanID).To(Equal(hs.P.ID))
			Expect(h2.ProviderTemplateID).To(Equal(hs.PT.ID))
		})

		It("fills up the fields in original struct", func() {
			h.Name = sql.NullString{"foo host", true}
			err := h.Update(db)
			Expect(err).To(BeNil())

			Expect(h.CreatedAt).NotTo(BeNil())
			Expect(h.UpdatedAt).NotTo(BeNil())
		})

		It("updates UpdatedAt", func() {
			origUpdatedAt := h.UpdatedAt

			h.Name = sql.NullString{"foo host", true}
			err := h.Update(db)
			Expect(err).To(BeNil())

			Expect(h.UpdatedAt).NotTo(Equal(origUpdatedAt))
		})
	})

	Describe("UpdateAfterRespec()", func() {
		var h *host.Host

		BeforeEach(func() {
			var err error
			h = &host.Host{
				Slug:               "host-1",
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              host.StateRespeccing,
				HourlyPrice:        100,
				TerminatedAt:       nil,
				Preprovisioned:     false,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates only the provider template ID, state, and IP address", func() {
			hs2, err := test.CreateHostStack(db, nil)
			Expect(err).To(BeNil())

			h.ProviderTemplateID = hs2.PT.ID
			h.State = host.StateRunning
			h.IPAddress = sql.NullString{"192.168.0.1", true}

			h.Slug = "new-slug"
			h.Name = sql.NullString{"new name", true}
			h.HourlyPrice = 200

			err = h.UpdateAfterRespec(db)
			Expect(err).To(BeNil())

			h2 := &host.Host{}
			err = db.Get(h2, `SELECT * FROM hosts WHERE id = $1`, h.ID)
			Expect(err).To(BeNil())
			Expect(h2).NotTo(BeNil())

			// Updated fields.
			Expect(h2.ProviderTemplateID).To(Equal(hs2.PT.ID))
			Expect(h2.State).To(Equal(host.StateRunning))
			Expect(h2.IPAddress.String).To(Equal("192.168.0.1"))

			// Unchanged fields.
			Expect(h2.Slug).To(Equal("host-1"))
			Expect(h2.UserID).To(Equal(hs.U.ID))
			Expect(h2.PlanID).To(Equal(h.PlanID))
			Expect(h2.RegionID).To(Equal(h.RegionID))
			Expect(h2.HourlyPrice).To(Equal(h.HourlyPrice))
			Expect(h2.ProviderIdentifier.String).To(Equal(h.ProviderIdentifier.String))
			Expect(h2.MaxContainers.Int64).To(Equal(h.MaxContainers.Int64))
			Expect(h2.FreighterdPort.Int64).To(Equal(h.FreighterdPort.Int64))
			Expect(h2.Preprovisioned).To(Equal(h.Preprovisioned))
			Expect(h2.FreighterdVersion).To(Equal(h.FreighterdVersion))
			Expect(h2.DockerVersion).To(Equal(h.DockerVersion))
		})

		It("updates UpdatedAt", func() {
			origUpdatedAt := h.UpdatedAt

			err := h.UpdateAfterRespec(db)
			Expect(err).To(BeNil())

			Expect(*h.UpdatedAt).To(BeTemporally(">", *origUpdatedAt))
		})
	})

	Describe("UpdateVersions()", func() {
		var (
			h    *host.Host
			slug = "foo"
		)

		BeforeEach(func() {
			var err error
			h = &host.Host{
				Slug:               slug,
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              host.StateRunning,
				TerminatedAt:       nil,
				Preprovisioned:     false,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the host's versions in the database", func() {
			fv := "2.0.88-badbed-55"
			dv := "2.6.9-nitrous"
			err := h.UpdateVersions(db, fv, dv)
			Expect(err).To(BeNil())

			h2 := &host.Host{}
			err = db.Get(h2, `SELECT * FROM hosts WHERE id = $1`, h.ID)
			Expect(err).To(BeNil())
			Expect(h2).NotTo(BeNil())

			// Updated fields.
			Expect(h2.FreighterdVersion).To(Equal("2.0.88-badbed-55"))
			Expect(h2.DockerVersion).To(Equal("2.6.9-nitrous"))

			// Unchanged fields.
			Expect(h2.UserID).To(Equal(hs.U.ID))
			Expect(h2.Slug).To(Equal("foo"))
			Expect(h2.RegionID).To(Equal(hs.R.ID))
			Expect(h2.PlanID).To(Equal(hs.P.ID))
			Expect(h2.ProviderTemplateID).To(Equal(hs.PT.ID))
		})
	})

	Describe("Reassign()", func() {
		It("updates the host's user_id to the given user's ID and the assigned_at timestamp", func() {
			h := &host.Host{
				Slug:               "foo",
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			err := h.Insert(db)
			Expect(err).To(BeNil())
			u := &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = test.WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			err = h.Reassign(db, u)
			Expect(err).To(BeNil())
			Expect(h.UserID).To(Equal(u.ID))
			Expect(h.AssignedAt).NotTo(BeNil())
		})
	})

	Describe("Delete()", func() {
		It("sets terminated_at instead of removing the host from the database", func() {
			h := &host.Host{
				Slug:               "foo",
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              host.StateRunning,
				TerminatedAt:       nil,
			}
			err := h.Insert(db)
			Expect(err).To(BeNil())

			err = h.Delete(db)
			Expect(err).To(BeNil())
			Expect(h.TerminatedAt).NotTo(BeNil())
			Expect(h.State).To(Equal(host.StateTerminated))

			h2 := &host.Host{}
			err = db.QueryRowx("SELECT * FROM hosts WHERE id = $1", h.ID).StructScan(h2)
			Expect(err).To(BeNil())
			Expect(h2).To(Equal(h))
		})
	})

	Describe("CanCreateMoreContainers()", func() {

		BeforeEach(func() {
			c := &container.Container{
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container",
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			c2 := &container.Container{
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "terminated-h",
			}
			err = c2.Insert(db)
			Expect(err).To(BeNil())
			err = c2.Delete(db)
		})

		It("should be false if the host has exceeded max_containers on the plan", func() {
			hs.H.MaxContainers = sql.NullInt64{1, true}
			state, err := hs.H.CanCreateMoreContainers(db, hs.U)
			Expect(err).To(BeNil())
			Expect(state).To(BeFalse())
		})

		It("should be true if the host has not exceeded max_containers on the plan", func() {
			hs.H.MaxContainers = sql.NullInt64{2, true}
			state, err := hs.H.CanCreateMoreContainers(db, hs.U)
			Expect(err).To(BeNil())
			Expect(state).To(BeTrue())
		})

		It("should be true if the host has NULL max_containers on the plan", func() {
			hs.H.MaxContainers = sql.NullInt64{0, false}
			state, err := hs.H.CanCreateMoreContainers(db, hs.U)
			Expect(err).To(BeNil())
			Expect(state).To(BeTrue())
		})

		It("should be true if the user has a higher max_containers limit", func() {
			hs.H.MaxContainers = sql.NullInt64{1, true}
			hs.U.MaxContainers = sql.NullInt64{2, true}
			state, err := hs.H.CanCreateMoreContainers(db, hs.U)
			Expect(err).To(BeNil())
			Expect(state).To(BeTrue())
		})

		It("should be false if the user does not have a higher max_containers limit", func() {
			hs.H.MaxContainers = sql.NullInt64{1, true}
			hs.U.MaxContainers = sql.NullInt64{1, true}
			state, err := hs.H.CanCreateMoreContainers(db, hs.U)
			Expect(err).To(BeNil())
			Expect(state).To(BeFalse())
		})

	})

	Describe("Validate()", func() {
		var (
			h *host.Host
		)

		BeforeEach(func() {
			h = &host.Host{
				UserID:             hs.U.ID,
				Slug:               "my-host-1",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			Expect(h.Validate()).To(BeNil())
		})

		It("validates name", func() {
			buf := make([]byte, 64)
			for i := range buf {
				buf[i] = 'f'
			}
			s := string(buf) // "f" * 64.

			tests := []struct {
				input string
				err   error
			}{
				{"", host.ErrSlugBlank},
				{"foo", nil},
				{"fo", nil},
				{"f", host.ErrSlugTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{s, host.ErrSlugTooLong},
				{s[:63], nil},
				{"f00", nil},
				{"f-o-0", nil},
				{"f-o-0-", host.ErrSlugInvalid},
				{"-foo", host.ErrSlugInvalid},
				{"foo_bar", host.ErrSlugInvalid},
				{"FOO", host.ErrSlugInvalid},
			}

			for _, t := range tests {
				h.Slug = t.input
				Expect(h.Validate() == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})

	Describe(".Hostname", func() {
		It("return the host DNS name", func() {
			prev := envvars.HostHostname
			envvars.HostHostname = "n2otest.com"
			defer func() { envvars.HostHostname = prev }()
			h := &host.Host{Slug: "hello"}
			Expect(h.Hostname()).To(Equal("hello.n2otest.com"))
		})
	})

	Describe(".CertHostname", func() {
		It("return the wildcard DNS name", func() {
			prev := envvars.HostHostname
			envvars.HostHostname = "n2otest.com"
			defer func() { envvars.HostHostname = prev }()
			h := &host.Host{Slug: "hello"}
			Expect(h.CertHostname()).To(Equal("*.n2otest.com"))
		})
	})

	Describe(".ProxyHostname", func() {
		It("return the host DNS name", func() {
			prev := envvars.ProxyHostname
			envvars.ProxyHostname = "n2otestproxy.com"
			defer func() { envvars.ProxyHostname = prev }()
			h := &host.Host{Slug: "world"}
			Expect(h.ProxyHostname()).To(Equal("world.n2otestproxy.com"))
		})
	})

	Describe(".MachineConfig", func() {
		It("returns configuration for the actual machine", func() {
			testCases := map[*host.Host]hostbuilderclient.Config{
				&host.Host{
					ProviderIdentifier:        sql.NullString{"i-123456", true},
					ProviderStorageIdentifier: sql.NullString{"vol-abcde", true},
					RegionIdentifier:          sql.NullString{"us-west-2", true},
					IPAddress:                 sql.NullString{"10.0.0.1", true},
				}: hostbuilderclient.Config{
					"id":          "i-123456",
					"volume_id":   "vol-abcde",
					"region_id":   "us-west-2",
					"ip_address":  "10.0.0.1",
					"disable_eip": false,
				},
				&host.Host{
					ProviderIdentifier:        sql.NullString{"i-123456", true},
					ProviderStorageIdentifier: sql.NullString{},
					RegionIdentifier:          sql.NullString{"us-west-2", true},
					IPAddress:                 sql.NullString{"10.0.0.1", true},
					ProviderTemplate:          &host.ProviderTemplate{},
				}: hostbuilderclient.Config{
					"id":          "i-123456",
					"volume_id":   "",
					"region_id":   "us-west-2",
					"ip_address":  "10.0.0.1",
					"disable_eip": false,
				},
				&host.Host{
					ProviderIdentifier:        sql.NullString{"i-123456", true},
					ProviderStorageIdentifier: sql.NullString{"vol-abcde", true},
					RegionIdentifier:          sql.NullString{"us-west-2", true},
					IPAddress:                 sql.NullString{"10.0.0.1", true},
					ProviderTemplate: &host.ProviderTemplate{
						Config: hstore.Hstore{
							Map: map[string]sql.NullString{
								"disable_eip": sql.NullString{"true", true},
							},
						},
					},
				}: hostbuilderclient.Config{
					"id":          "i-123456",
					"volume_id":   "vol-abcde",
					"region_id":   "us-west-2",
					"ip_address":  "10.0.0.1",
					"disable_eip": true,
				},
			}

			for h, cfg := range testCases {
				Expect(h.MachineConfig()).To(Equal(cfg))
			}
		})
	})
})

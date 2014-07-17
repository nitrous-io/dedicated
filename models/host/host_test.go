package host_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/user"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/host")
}

var _ = Describe("models/host", func() {
	var (
		db *db.Database
		hs *HostStack
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, nil)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAllByUser()", func() {
		var (
			u1 *user.User
			u2 *user.User
		)

		BeforeEach(func() {
			var err error
			u1 = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = u1.Insert(db)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err = u2.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO hosts
				(user_id, name, region_id, plan_id, provider_template_id)
				VALUES ($1, $2, $3, $4, $5);`,
				u1.ID, "my-host-1", hs.R.ID, hs.P.ID, hs.PT.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts (user_id, name, region_id, plan_id, provider_template_id)
				VALUES ($1, $2, $3, $4, $5);`,
				u1.ID, "my-host-2", hs.R.ID, hs.P.ID, hs.PT.ID)
			Expect(err).To(BeNil())
		})

		Context("when the given user has host records", func() {
			It("returns the host records", func() {
				hosts, err := host.FindAllByUser(db, u1)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(2))

				Expect(hosts[0].ID).NotTo(BeZero())
				Expect(hosts[0].UserID).To(Equal(u1.ID))
				Expect(hosts[0].Name).To(Equal("my-host-1"))

				Expect(hosts[1].ID).NotTo(BeZero())
				Expect(hosts[1].UserID).To(Equal(u1.ID))
				Expect(hosts[1].Name).To(Equal("my-host-2"))
			})
		})

		Context("when the given user does not have any host records", func() {
			It("returns an empty slice", func() {
				hosts, err := host.FindAllByUser(db, u2)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(0))
			})
		})
	})

	Describe("FindByUserAndName()", func() {
		Context("when there's a host record for the given name that belongs to the user", func() {
			It("returns the host record matching the name", func() {
				h, err := host.FindByUserAndName(db, hs.U, hs.H.Name)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.UserID).To(Equal(hs.U.ID))
				Expect(h.Name).To(Equal(hs.H.Name))
				Expect(h.Provider).To(Equal(hs.PT.Provider))
			})
		})

		Context("when there's a host record for the given name that does not belong to the user", func() {
			var u *user.User
			BeforeEach(func() {
				u = &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := u.Insert(db)
				Expect(err).To(BeNil())
			})

			It("returns nil without an error", func() {
				h, err := host.FindByUserAndName(db, u, hs.H.Name)
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})
		})

		Context("when a host record with the given name does not exist", func() {
			It("returns nil without an error", func() {
				h, err := host.FindByUserAndName(db, hs.U, "vivabrazil")
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				h, err := host.FindByUserAndName(db, hs.U, hs.H.Name)
				Expect(h).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var (
			h *host.Host
			u *user.User
		)

		BeforeEach(func() {
			var err error
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = u.Insert(db)
			Expect(err).To(BeNil())

			h = &host.Host{
				UserID:             u.ID,
				Name:               "my-host-1",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the host in the database", func() {
			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			Expect(hosts[0].ID).NotTo(BeZero())
			Expect(hosts[0].UserID).To(Equal(u.ID))
			Expect(hosts[0].Name).To(Equal("my-host-1"))
			Expect(hosts[0].CreatedAt).NotTo(BeNil())
			Expect(hosts[0].UpdatedAt).NotTo(BeNil())
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(h.ID).NotTo(BeZero())
			Expect(h.CreatedAt).NotTo(BeNil())
			Expect(h.UpdatedAt).NotTo(BeNil())
		})

		It("sets state to a default of 'born'", func() {
			Expect(h.State).To(Equal("born"))
		})
	})

	Describe("UpdateState()", func() {
		var (
			h   *host.Host
			err error
		)

		BeforeEach(func() {
			h = &host.Host{
				Name:               "foo",
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              "running",
			}

			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("changes the state column in host record", func() {
			err := h.UpdateState(db, "stopped")
			Expect(err).To(BeNil())
			Expect(h.State).To(Equal("stopped"))

			h1, err := host.FindByUserAndName(db, hs.U, "foo")
			Expect(err).To(BeNil())
			Expect(h1.State).To(Equal("stopped"))
		})
	})

	Describe("Delete()", func() {
		var (
			h *host.Host
		)

		BeforeEach(func() {
			h = &host.Host{
				Name:               "foo",
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              "running",
				TerminatedAt:       nil,
			}
			err := h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("sets terminated_at instead of removing the host from the database", func() {
			err := h.Delete(db)
			Expect(err).To(BeNil())
			Expect(h.TerminatedAt).NotTo(BeNil())

			h2 := &host.Host{}
			err = db.QueryRowx("SELECT * FROM hosts WHERE id = $1", h.ID).StructScan(h2)
			Expect(err).To(BeNil())
			Expect(h2).To(Equal(h))
		})

		It("fills up the fields in the struct", func() {
			h2 := &host.Host{ID: h.ID}
			err := h2.Delete(db)
			Expect(err).To(BeNil())

			Expect(h2.CreatedAt).NotTo(BeNil())
			Expect(h2.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Validate()", func() {
		var (
			h *host.Host
		)

		BeforeEach(func() {
			h = &host.Host{
				UserID:             hs.U.ID,
				Name:               "my-host-1",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			Expect(h.Validate(db)).To(BeNil())
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
				{"", host.ErrNameBlank},
				{"foo", nil},
				{"fo", nil},
				{"f", host.ErrNameTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{s, host.ErrNameTooLong},
				{s[:63], nil},
				{"f00", nil},
				{"f-o-0-", nil},
				{"-foo", host.ErrNameInvalid},
				{"FOO", host.ErrNameInvalid},
			}

			for _, t := range tests {
				h.Name = t.input
				Expect(h.Validate(db) == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})
})

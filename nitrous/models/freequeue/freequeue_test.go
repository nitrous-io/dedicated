package freequeue_test

import (
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/freequeue"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/freequeue")
}

var _ = Describe("models/freequeue", func() {
	var (
		u1, u2, u3 *user.User
		admin      *user.User
		db         *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		admin = &user.User{
			Username: "foo",
			Email:    "foo@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, admin.Insert)
		Expect(err).To(BeNil())

		u1 = &user.User{
			Username: "bar",
			Email:    "bar@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u1.Insert)
		Expect(err).To(BeNil())

		u2 = &user.User{
			Username: "baz",
			Email:    "baz@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u2.Insert)
		Expect(err).To(BeNil())

		u3 = &user.User{
			Username: "bob",
			Email:    "bob@example.com",
			Password: "123456",
		}
		err = test.WrapInTransaction(db, u3.Insert)
		Expect(err).To(BeNil())
	})

	var insert = func(f *freequeue.FreeQueue) error {
		nstmt, err := db.PrepareNamed(`INSERT INTO free_queue
    (user_id, container_template_slug, container_name, host_name, region_slug)
    VALUES (:user_id, :container_template_slug, :container_name, :host_name, :region_slug)
    RETURNING *;`)
		if err != nil {
			return err
		}
		return nstmt.QueryRow(f).StructScan(f)
	}

	Describe("Cancel()", func() {
		It("returns should soft delete the free_queue assigned to the user", func() {
			f1 := &freequeue.FreeQueue{
				UserID:                u1.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(f1)).To(BeNil())

			f2 := &freequeue.FreeQueue{
				UserID:                u2.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(f2)).To(BeNil())

			Expect(freequeue.Cancel(db, u1.ID)).To(BeNil())
			_, err := freequeue.Find(db, u1.ID)
			Expect(err).To(Equal(sql.ErrNoRows))

			fq := &freequeue.FreeQueue{}
			Expect(db.Get(fq, `SELECT * FROM free_queue WHERE id = $1;`, f1.ID)).To(BeNil())
			Expect(fq.ID).To(Equal(f1.ID))

			_, err = freequeue.Find(db, u2.ID)
			Expect(err).To(BeNil())
		})
	})

	Describe("Find()", func() {
		It("returns a user's free queue record", func() {
			f := &freequeue.FreeQueue{
				UserID:                u1.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(f)).To(BeNil())

			fq, err := freequeue.Find(db, u1.ID)
			Expect(err).To(BeNil())
			Expect(fq.UserID).To(Equal(u1.ID))
			Expect(fq.ContainerTemplateSlug).To(Equal("tmpl1"))
			Expect(fq.ContainerName).To(Equal("nm"))
			Expect(fq.HostName).To(Equal("my workspace"))
			Expect(fq.RegionSlug).To(Equal("slug1"))
		})

		It("returns an error if user has no pending host", func() {
			_, err := freequeue.Find(db, u1.ID)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("finding by state", func() {
		var fq1, fq2, fq3 *freequeue.FreeQueue

		BeforeEach(func() {
			fq1 = &freequeue.FreeQueue{
				UserID:                u1.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(fq1)).To(BeNil())

			fq2 = &freequeue.FreeQueue{
				UserID:                u2.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(fq2)).To(BeNil())

			fq3 = &freequeue.FreeQueue{
				UserID:                u3.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(fq3)).To(BeNil())

			_, err := db.Exec("UPDATE free_queue SET state = 'approved' WHERE id = $1", fq3.ID)
			Expect(err).To(BeNil())
			err = db.Get(fq3, "SELECT * FROM free_queue WHERE id = $1", fq3.ID)
			Expect(err).To(BeNil())
		})

		Describe("FindAllPending()", func() {
			It("should find pending", func() {
				res, err := freequeue.FindAllPending(db)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(2))
				Expect([]freequeue.FreeQueue{res[0].FreeQueue, res[1].FreeQueue}).To(ConsistOf([]freequeue.FreeQueue{*fq1, *fq2}))
				Expect([]string{res[0].Username, res[1].Username}).To(ConsistOf([]string{"bar", "baz"}))
			})

			It("should not include deleted users", func() {
				_, err := db.Exec("UPDATE users SET deleted_at = now() WHERE id = $1", u2.ID)
				Expect(err).To(BeNil())

				res, err := freequeue.FindAllPending(db)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				Expect(res[0].FreeQueue).To(Equal(*fq1))
				Expect(res[0].Username).To(Equal("bar"))
			})
		})

		Describe("FindAllApproved()", func() {
			It("should find approved", func() {
				res, err := freequeue.FindAllApproved(db)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(1))
				Expect([]freequeue.FreeQueue{res[0].FreeQueue}).To(Equal([]freequeue.FreeQueue{*fq3}))
				Expect([]string{res[0].Username}).To(Equal([]string{"bob"}))
			})

			It("should not include deleted users", func() {
				_, err := db.Exec("UPDATE users SET deleted_at = now() WHERE id = $1", u3.ID)
				Expect(err).To(BeNil())

				res, err := freequeue.FindAllApproved(db)
				Expect(err).To(BeNil())
				Expect(res).To(HaveLen(0))
			})
		})
	})

	Describe("Action()", func() {
		var (
			fq1, fq2, fq3 *freequeue.FreeQueue
			tx            *sqlx.Tx
		)

		BeforeEach(func() {
			fq1 = &freequeue.FreeQueue{
				UserID:                u1.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(fq1)).To(BeNil())

			fq2 = &freequeue.FreeQueue{
				UserID:                u2.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(fq2)).To(BeNil())

			fq3 = &freequeue.FreeQueue{
				UserID:                u3.ID,
				ContainerTemplateSlug: "tmpl1",
				ContainerName:         "nm",
				HostName:              "my workspace",
				RegionSlug:            "slug1",
			}
			Expect(insert(fq3)).To(BeNil())

			_, err := db.Exec("UPDATE free_queue SET state = 'approved' WHERE id = $1", fq3.ID)
			Expect(err).To(BeNil())
			err = db.Get(fq3, "SELECT * FROM free_queue WHERE id = $1", fq3.ID)
			Expect(err).To(BeNil())

			tx, _ = db.Beginx()
		})

		AfterEach(func() {
			tx.Rollback()
		})

		It("should require a valid state", func() {
			_, err := freequeue.Action(tx, fq1.ID, "", "reboot", admin.ID)
			Expect(err).To(Equal(freequeue.ErrInvalidState))
		})

		It("should update successfully", func() {
			fq, err := freequeue.Action(tx, fq1.ID, fq1.State, freequeue.StateApproved, admin.ID)
			Expect(err).To(BeNil())

			err = tx.Get(fq1, "SELECT * FROM free_queue WHERE id = $1", fq1.ID)
			Expect(err).To(BeNil())
			Expect(fq1.State).To(Equal(freequeue.StateApproved))
			Expect(fq1.ActionedBy).To(Equal(sql.NullInt64{admin.ID, true}))
			Expect(fq1.ActionedAt.Valid).To(BeTrue())
			Expect(fq).To(Equal(fq1))
		})

		It("should not allow the states to be out of sync", func() {
			_, err := freequeue.Action(tx, fq1.ID, freequeue.StateApproved, freequeue.StateRejected, admin.ID)
			Expect(err).To(Equal(freequeue.ErrInconsistentState))

			err = tx.Get(fq1, "SELECT * FROM free_queue WHERE id = $1", fq1.ID)
			Expect(err).To(BeNil())
			Expect(fq1.State).To(Equal(freequeue.StatePending))
			Expect(fq1.ActionedBy.Valid).To(BeFalse())
			Expect(fq1.ActionedAt.Valid).To(BeFalse())
		})

		It("should not allow it to be actioned twice", func() {
			_, err := freequeue.Action(tx, fq1.ID, fq1.State, freequeue.StateRejected, admin.ID)
			Expect(err).To(BeNil())

			_, err = freequeue.Action(tx, fq1.ID, fq1.State, freequeue.StateApproved, admin.ID)
			Expect(err).To(Equal(freequeue.ErrInconsistentState))
		})
	})

})

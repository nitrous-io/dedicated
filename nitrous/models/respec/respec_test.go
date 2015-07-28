package respec_test

import (
	"database/sql"
	"testing"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/respec"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/respec")
}

var _ = Describe("models/respec", func() {
	var (
		hs1, hs2 *test.HostStack
		db       *ndb.Database
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs1, err = test.CreateHostStack(db, nil)
		Expect(err).To(BeNil())

		// Create hoststack in same region.
		hs2, err = test.CreateHostStack(db, &test.HostStack{
			R: hs1.R,
		})
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Find()", func() {
		It("returns the pending respec of the given host", func() {
			_, err := db.Exec(`INSERT INTO respecs (host_id, provider_template_id, state)
				VALUES ($1, $2, $3);`, hs1.H.ID, hs2.PT.ID, respec.StatePending)
			Expect(err).To(BeNil())

			r, err := respec.Find(db, hs1.H.ID)
			Expect(err).To(BeNil())
			Expect(r.HostID).To(Equal(hs1.H.ID))
			Expect(r.ProviderTemplateID).To(Equal(hs2.PT.ID))
			Expect(r.State).To(Equal(respec.StatePending))
		})

		It("returns an error when host has no pending respecs", func() {
			_, err := respec.Find(db, hs1.H.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("does not return respecs that are not in pending state", func() {
			_, err := db.Exec(`INSERT INTO respecs (host_id, provider_template_id, state)
				VALUES ($1, $2, $3);`, hs1.H.ID, hs1.PT.ID, respec.StateCompleted)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO respecs (host_id, provider_template_id, state)
				VALUES ($1, $2, $3);`, hs1.H.ID, hs2.PT.ID, respec.StateProcessing)
			Expect(err).To(BeNil())

			_, err = respec.Find(db, hs1.H.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})
	})

	Describe("FindProcessing()", func() {
		It("returns the processing respec of the given host", func() {
			_, err := db.Exec(`INSERT INTO respecs (host_id, provider_template_id, state)
				VALUES ($1, $2, $3);`, hs1.H.ID, hs2.PT.ID, respec.StateProcessing)
			Expect(err).To(BeNil())

			r, err := respec.FindProcessing(db, hs1.H.ID)
			Expect(err).To(BeNil())
			Expect(r.HostID).To(Equal(hs1.H.ID))
			Expect(r.ProviderTemplateID).To(Equal(hs2.PT.ID))
			Expect(r.State).To(Equal(respec.StateProcessing))
		})

		It("returns an error when host has no processing respecs", func() {
			_, err := respec.FindProcessing(db, hs1.H.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})

		It("does not return respecs that are not in processing state", func() {
			_, err := db.Exec(`INSERT INTO respecs (host_id, provider_template_id, state)
				VALUES ($1, $2, $3);`, hs1.H.ID, hs1.PT.ID, respec.StatePending)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO respecs (host_id, provider_template_id, state)
				VALUES ($1, $2, $3);`, hs1.H.ID, hs2.PT.ID, respec.StateCompleted)
			Expect(err).To(BeNil())

			_, err = respec.FindProcessing(db, hs1.H.ID)
			Expect(err).To(Equal(sql.ErrNoRows))
		})
	})

	findAll := func(d *ndb.Database, hostID int64) ([]*respec.Respec, error) {
		respecs := []*respec.Respec{}
		err := d.Select(&respecs, `SELECT * FROM respecs WHERE host_id = $1`, hostID)
		if err != nil {
			return nil, err
		}
		return respecs, nil
	}

	Describe("Insert()", func() {
		var r *respec.Respec

		BeforeEach(func() {
			r = &respec.Respec{
				HostID:             hs1.H.ID,
				ProviderTemplateID: hs2.PT.ID,
			}
		})

		It("persists the respec in the database", func() {
			err := db.WithTransaction(r.Insert)
			Expect(err).To(BeNil())

			rs, err := findAll(db, hs1.H.ID)
			Expect(err).To(BeNil())
			Expect(rs).To(HaveLen(1))

			Expect(rs[0].HostID).To(Equal(hs1.H.ID))
			Expect(rs[0].ProviderTemplateID).To(Equal(hs2.PT.ID))
			Expect(rs[0].State).To(Equal(respec.StatePending))

			Expect(rs[0].CreatedAt).NotTo(BeNil())
			Expect(rs[0].UpdatedAt).NotTo(BeNil())
		})

		It("changes all the host's pending respecs to obsolete", func() {
			_, err := db.Exec(`INSERT INTO respecs (host_id, provider_template_id, state)
				VALUES ($1, $2, $3);`, hs1.H.ID, hs2.PT.ID, respec.StatePending)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO respecs (host_id, provider_template_id, state)
				VALUES ($1, $2, $3);`, hs1.H.ID, hs1.PT.ID, respec.StateCompleted)
			Expect(err).To(BeNil())

			err = db.WithTransaction(r.Insert)
			Expect(err).To(BeNil())

			rs, err := findAll(db, hs1.H.ID)
			Expect(err).To(BeNil())
			Expect(rs).To(HaveLen(3))

			var pending, obsolete, completed *respec.Respec
			for _, r := range rs {
				switch r.State {
				case respec.StatePending:
					pending = r
				case respec.StateObsolete:
					obsolete = r
				case respec.StateCompleted:
					completed = r
				}
			}

			Expect(pending).NotTo(BeNil())
			Expect(pending.HostID).To(Equal(hs1.H.ID))
			Expect(pending.ProviderTemplateID).To(Equal(hs2.PT.ID))

			// Previously pending respec should become obsolete.
			Expect(obsolete).NotTo(BeNil())
			Expect(obsolete.HostID).To(Equal(hs1.H.ID))
			Expect(obsolete.ProviderTemplateID).To(Equal(hs2.PT.ID))

			// Completed respec does not change.
			Expect(completed).NotTo(BeNil())
			Expect(completed.HostID).To(Equal(hs1.H.ID))
			Expect(completed.ProviderTemplateID).To(Equal(hs1.PT.ID))
		})
	})

	Describe("UpdateState()", func() {
		It("updates the respec's state to the given state", func() {
			rs := respec.Respec{
				HostID:             hs1.H.ID,
				ProviderTemplateID: hs2.PT.ID,
				State:              respec.StatePending,
			}

			err := db.WithTransaction(rs.Insert)
			Expect(err).To(BeNil())

			origUpdatedAt := rs.UpdatedAt
			Expect(origUpdatedAt.Valid).To(BeTrue())

			err = rs.UpdateState(db, respec.StateCompleted)
			Expect(err).To(BeNil())
			Expect(rs.State).To(Equal(respec.StateCompleted))
			Expect(rs.UpdatedAt.Time).To(BeTemporally(">", origUpdatedAt.Time))
		})
	})
})

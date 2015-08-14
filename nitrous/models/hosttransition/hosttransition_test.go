package hosttransition_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/hosttransition")
}

var _ = Describe("models/hosttransition", func() {
	var (
		db  *db.Database
		u   *user.User
		hs1 *HostStack
		h1  *host.Host
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

		hs1, err = CreateHostStack(db, &HostStack{
			H: &host.Host{
				Slug:        "my-host-1",
				HourlyPrice: 3000,
			},
			U: u,
		})
		Expect(err).To(BeNil())
		h1 = hs1.H
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe(".Record", func() {

		Context("with no existing transition for host", func() {
			var (
				exId int64
				oH   *host.Host
			)
			BeforeEach(func() {
				oHs, err := CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:        "my-host-2",
						HourlyPrice: 9000,
					},
					P:  hs1.P,
					R:  hs1.R,
					PT: hs1.PT,
					U:  u,
				})
				Expect(err).To(BeNil())
				oH = oHs.H

				stm, err := db.Prepare(
					`INSERT INTO host_transitions (host_id, previous_state, state) VALUES ($1, 'restarting', 'suspended')  RETURNING ID`)
				Expect(err).To(BeNil())

				err = stm.QueryRow(oH.ID).Scan(&exId)
				Expect(err).To(BeNil())
			})

			It("should insert without touching other transitions", func() {
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				t := time.Now()
				h1.State = "terminated"
				err = hosttransition.Record(tx, h1, "running", t, u.ID)
				Expect(err).To(BeNil())
				Expect(tx.Commit()).To(BeNil())

				preEx, err := hosttransition.FindByID(db, exId)
				Expect(err).To(BeNil())

				Expect(preEx.HostID).To(Equal(oH.ID))
				Expect(preEx.PreviousState.String).To(Equal("restarting"))
				Expect(preEx.State).To(Equal("suspended"))

				created, err := hosttransition.FindMostRecentForHost(db, h1.ID)
				Expect(err).To(BeNil())
				Expect(created).ToNot(BeNil())

				Expect(created.State).To(Equal("terminated"))
				Expect(created.PreviousState.String).To(Equal("running"))
				Expect(created.CreatedAt.Unix()).To(Equal(t.Unix()))
				Expect(created.InitiatorID).To(Equal(sql.NullInt64{u.ID, true}))
			})

			It("should update the host running if the state becomes running", func() {
				Expect(h1.LastRunningAt.Valid).To(BeFalse())
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				t := time.Now()
				h1.State = "running"
				err = hosttransition.Record(tx, h1, "born", t, u.ID)
				Expect(err).To(BeNil())
				Expect(tx.Commit()).To(BeNil())

				h, err := host.FindByIdentifier(db, "my-host-1")
				Expect(err).To(BeNil())
				Expect(h.LastRunningAt.Valid).To(BeTrue())
			})

			It("should flush out idle_notices_sent and idle_next_check if the state becomes running", func() {
				_, err := db.Exec("UPDATE hosts SET idle_next_check = now(), idle_notices_sent = 2 WHERE id = $1", h1.ID)
				Expect(err).To(BeNil())

				h1, err = host.FindByIdentifier(db, "my-host-1")
				Expect(h1.IdleNextCheck.Valid).To(BeTrue())
				Expect(h1.IdleNoticesSent).To(Equal(2))

				tx, err := db.Beginx()
				Expect(err).To(BeNil())
				t := time.Now()
				h1.State = "running"
				err = hosttransition.Record(tx, h1, "born", t, u.ID)
				Expect(err).To(BeNil())
				Expect(tx.Commit()).To(BeNil())

				h, err := host.FindByIdentifier(db, "my-host-1")
				Expect(err).To(BeNil())
				Expect(h.IdleNextCheck.Valid).To(BeFalse())
				Expect(h.IdleNoticesSent).To(BeZero())
			})

			It("should not update the host running if the state becomes running", func() {
				Expect(h1.LastRunningAt.Valid).To(BeFalse())
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				t := time.Now()
				h1.State = "terminated"
				err = hosttransition.Record(tx, h1, "running", t, u.ID)
				Expect(err).To(BeNil())
				Expect(tx.Commit()).To(BeNil())

				h, err := host.FindByIdentifier(db, "my-host-1")
				Expect(err).To(BeNil())
				Expect(h.LastRunningAt.Valid).To(BeFalse())
			})
		})

		Context("with existing transitions for host", func() {
			var (
				exId int64
			)
			BeforeEach(func() {
				stm, err := db.Prepare(
					`INSERT INTO host_transitions (host_id, previous_state, state) VALUES ($1, 'restarting', 'suspended')  RETURNING ID`)
				Expect(err).To(BeNil())

				err = stm.QueryRow(h1.ID).Scan(&exId)
				Expect(err).To(BeNil())
			})

			It("should update existing transition's ended_at for host", func() {
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				t := time.Now()
				err = hosttransition.Record(tx, h1, "running", t, u.ID)
				Expect(err).To(BeNil())
				Expect(tx.Commit()).To(BeNil())

				preEx, err := hosttransition.FindByID(db, exId)
				Expect(err).To(BeNil())

				Expect(preEx.PreviousState.String).To(Equal("restarting"))
				Expect(preEx.State).To(Equal("suspended"))
				Expect(preEx.EndedAt.Unix()).To(Equal(t.Unix()))

				created, err := hosttransition.FindMostRecentForHost(db, h1.ID)
				Expect(err).To(BeNil())
				Expect(created).ToNot(BeNil())

				Expect(created.CreatedAt).To(Equal(preEx.EndedAt))
				Expect(created.InitiatorID).To(Equal(sql.NullInt64{u.ID, true}))
			})
		})

	})

	Describe("UsageForHosts", func() {
		var (
			hs2 *HostStack
			hs3 *HostStack
			h2  *host.Host
			h3  *host.Host

			startTime time.Time
			endTime   time.Time
		)

		BeforeEach(func() {
			startTime = time.Date(2014, 1, 1, 0, 0, 0, 0, time.Local)
			endTime = time.Date(2014, 1, 31, 24, 59, 59, 1000000000, time.Local)

			_, err := db.Exec("UPDATE hosts SET terminated_at = $1, updated_at = now() WHERE id = $2;", endTime, h1.ID)
			Expect(err).To(BeNil())

			hs2, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-2",
					HourlyPrice: 500,
				},
				R:  hs1.R,
				PT: hs1.PT,
				U:  u,
			})
			Expect(err).To(BeNil())
			h2 = hs2.H

			hs3, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:        "my-host-3",
					HourlyPrice: 3722, // total cost for host will theoretically be $25.01 but plan price is 25.00
				},
				P: &plan.Plan{
					Slug:          "hs3Plan",
					Name:          "hs3Plan",
					MaxContainers: 1,
					Price:         2500,
				},
				R:  hs1.R,
				PT: hs1.PT,
				U:  u,
			})
			Expect(err).To(BeNil())
			h3 = hs3.H

			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h1.ID, "born", "running", startTime, startTime.Add(2*time.Hour))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h1.ID, "running", "stopped", startTime.Add(2*time.Hour))
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "born", "running", startTime, startTime.Add(2*time.Hour))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h2.ID, "running", "stopped", startTime.Add(2*time.Hour), startTime.Add(4*time.Hour))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h2.ID, "stopped", "running", startTime.Add(4*time.Hour))
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h3.ID, "born", "running", startTime)
			Expect(err).To(BeNil())
		})

		It("should list the hosts for times it is running", func() {
			times, err := hosttransition.UsageForHosts(db, []int64{h1.ID, h2.ID, h3.ID}, startTime, endTime)
			Expect(err).To(BeNil())
			Expect(times).To(HaveLen(3))

			hl1, ok := times[h1.ID]
			Expect(ok).To(BeTrue())
			Expect(hl1).To(HaveLen(1))
			Expect(hl1[0].Minutes).To(Equal(int64(120)))
			Expect(hl1[0].Start).To(BeTemporally("==", startTime))
			Expect(hl1[0].End).To(BeTemporally("==", startTime.Add(2*time.Hour)))

			hl2, ok := times[h2.ID]
			Expect(ok).To(BeTrue())
			Expect(hl2).To(HaveLen(2))
			Expect(hl2[0].Minutes).To(Equal(int64(120)))
			Expect(hl2[0].Start).To(BeTemporally("==", startTime))
			Expect(hl2[0].End).To(BeTemporally("==", startTime.Add(2*time.Hour)))
			Expect(hl2[1].Minutes).To(Equal(int64(44460)))
			Expect(hl2[1].Start).To(BeTemporally("==", startTime.Add(4*time.Hour)))
			Expect(hl2[1].End).To(BeTemporally("==", endTime))

			hl3, ok := times[h3.ID]
			Expect(ok).To(BeTrue())
			Expect(hl3).To(HaveLen(1))
			Expect(hl3[0].Minutes).To(Equal(int64(44700)))
			Expect(hl3[0].Start).To(BeTemporally("==", startTime))
			Expect(hl3[0].End).To(BeTemporally("==", endTime))
		})

		It("should round up part of a minute", func() {
			_, err := db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at, ended_at) values ($1, $2, $3, $4, $5);", h1.ID, "stopped", "running", startTime.Add(2*time.Hour), startTime.Add(140*time.Minute+1*time.Second))
			Expect(err).To(BeNil())
			_, err = db.Exec("INSERT INTO host_transitions (host_id, previous_state, state, created_at) values ($1, $2, $3, $4);", h1.ID, "running", "stopped", startTime.Add(140*time.Minute))
			Expect(err).To(BeNil())

			times, err := hosttransition.UsageForHosts(db, []int64{h1.ID}, startTime, endTime)
			Expect(err).To(BeNil())
			Expect(times).To(HaveLen(1))

			hl1, ok := times[h1.ID]
			Expect(ok).To(BeTrue())
			Expect(hl1).To(HaveLen(2))
			Expect(hl1[0].Minutes).To(Equal(int64(120)))
			Expect(hl1[0].Start).To(BeTemporally("==", startTime))
			Expect(hl1[0].End).To(BeTemporally("==", startTime.Add(2*time.Hour)))
			Expect(hl1[1].Minutes).To(Equal(int64(21)))
			Expect(hl1[1].Start).To(BeTemporally("==", startTime.Add(2*time.Hour)))
			Expect(hl1[1].End).To(BeTemporally("==", startTime.Add(140*time.Minute+1*time.Second)))
		})
	})

})

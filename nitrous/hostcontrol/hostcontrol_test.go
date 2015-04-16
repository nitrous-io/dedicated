package hostcontrol_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous-common/logger"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostcontrol")
}

var _ = Describe("hostcontrol", func() {
	var (
		db *ndb.Database
		l  *logger.Logger
		u  *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		l = &logger.Logger{Level: logger.LogLevelQuiet}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Stop()", func() {
		var (
			h *host.Host
		)

		BeforeEach(func() {
			var err error
			hs, err := test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					State:            host.StateRunning,
					IPAddress:        sql.NullString{"127.0.0.1", true},
					FreighterdPort:   sql.NullInt64{65333, true},
					RegionIdentifier: sql.NullString{"us-east-1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())

			h, err = host.FindByIdentifier(db, hs.H.Slug)
			Expect(err).To(BeNil())
		})

		It("should send the UUID headers", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: stopOkResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hostcontrol.HostbuilderClient = hb

			Expect(hostcontrol.Stop(db, l, h)).To(BeNil())

			req := rt.Requests[0]
			Expect(req.Header.Get(constants.NitrousUUIDHeader)).To(Equal("hostcontrol"))
		})

		It("sends the correct payload to hostbuilder", func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt := &test.FakeRoundTripper{
				Message: stopOkResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hostcontrol.HostbuilderClient = hb

			Expect(hostcontrol.Stop(db, l, h)).To(BeNil())

			payload := &struct {
				ID        string `json:"id"`
				IPAddress string `json:"ip_address"`
				Region    string `json:"region_id"`
			}{}
			json.Unmarshal([]byte(rt.Requests[0].FormValue("config")), payload)

			Expect(payload.ID).To(Equal(h.ProviderIdentifier.String), "invalid id")
			Expect(payload.IPAddress).To(Equal(h.IPAddress.String), "invalid ip_address")
			Expect(payload.Region).To(Equal("us-east-1"), "invalid region")
		})

		Context("when host is already stopped", func() {
			It("allows host to be stopped", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: stopOkResponse,
					Status:  http.StatusOK,
				})
				hostcontrol.HostbuilderClient = hb

				h.State = host.StateStopping
				err := h.Update(db)
				Expect(err).To(BeNil())

				Expect(hostcontrol.Stop(db, l, h)).To(BeNil())

				h, err = host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopped))
			})
		})

		Context("when hostbuilder successfully stops the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: stopOkResponse,
					Status:  http.StatusOK,
				})

				hostcontrol.HostbuilderClient = hb
			})

			It("updates the host state to stopped", func() {
				Expect(hostcontrol.Stop(db, l, h)).To(BeNil())

				var err error
				h, err = host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopped))
			})

			It("logs transitions for the host", func() {
				origState := h.State

				Expect(hostcontrol.Stop(db, l, h)).To(BeNil())

				var count int
				err := db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					h.ID, host.StateStopping, origState).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					h.ID, host.StateStopped, host.StateStopping).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: errorResponse,
					Status:  http.StatusBadRequest,
				})

				hostcontrol.HostbuilderClient = hb
			})

			It("resets the host to its original state", func() {
				Expect(hostcontrol.Stop(db, l, h)).ToNot(BeNil())

				var err error
				h, err = host.Find(db, u, h.Slug)
				Expect(err).To(BeNil())
				Expect(h.IPAddress.String).To(Equal("127.0.0.1"))
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("logs transitions for the host", func() {
				origState := h.State

				Expect(hostcontrol.Stop(db, l, h)).ToNot(BeNil())

				var count int
				err := db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					h.ID, host.StateStopping, origState).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))

				err = db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					h.ID, origState, host.StateStopping).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})
		})
	})

})

var (
	stopOkResponse = `{"state": "stopped", "ip_address": ""}`
	okResponse     = `{"freighterd_port":65333,"ip_address":"127.0.0.1"}`
	errorResponse  = `{"error":"some error occurred"}`
)

// replace the transport in hostbuilder client with a new fake transport
func replaceTransport(hb *hostbuilderclient.Hostbuilder, t http.RoundTripper) {
	hb.Client.Client.Transport = t
}

package hostcontrol_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jmoiron/sqlx"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func hbHeaders() http.Header {
	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, "hostcontrol")
	return headers
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostcontrol")
}

var _ = Describe("hostcontrol", func() {
	var (
		db *ndb.Database
		u  *user.User
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

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

	Describe("StopWithQueue()", func() {
		var (
			h      *host.Host
			events []hostcontrol.EventType
			rt     *test.FakeRoundTripper
		)

		var evCallback = func(e hostcontrol.EventType) {
			events = append(events, e)
		}

		BeforeEach(func() {
			events = []hostcontrol.EventType{}

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

			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt = &test.FakeRoundTripper{
				Message: stopOkResponse,
				Status:  http.StatusOK,
			}
			replaceTransport(hb, rt)
			hostcontrol.HostbuilderClient = hb
		})

		It("should send the UUID headers", func() {
			Expect(hostcontrol.StopWithQueue(db, h, hbHeaders(), evCallback, u.ID)).To(BeNil())

			req := rt.Requests[0]
			Expect(req.Header.Get(constants.NitrousUUIDHeader)).To(Equal("hostcontrol"))
			Expect(events).To(ConsistOf(hostcontrol.EventHostStopping))
		})

		It("sends the correct payload to hostbuilder", func() {
			Expect(hostcontrol.StopWithQueue(db, h, hbHeaders(), evCallback, u.ID)).To(BeNil())

			payload := &struct {
				ID        string `json:"id"`
				IPAddress string `json:"ip_address"`
				Region    string `json:"region_id"`
			}{}
			json.Unmarshal([]byte(rt.Requests[0].FormValue("config")), payload)

			Expect(payload.ID).To(Equal(h.ProviderIdentifier.String), "invalid id")
			Expect(payload.IPAddress).To(Equal(h.IPAddress.String), "invalid ip_address")
			Expect(payload.Region).To(Equal("us-east-1"), "invalid region")
			Expect(events).To(ConsistOf(hostcontrol.EventHostStopping))
		})

		It("updates the host state to stopping", func() {
			Expect(hostcontrol.StopWithQueue(db, h, hbHeaders(), evCallback, u.ID)).To(BeNil())

			var err error
			h, err = host.Find(db, u, h.Slug)
			Expect(err).To(BeNil())
			Expect(h.State).To(Equal(host.StateStopping))
			Expect(events).To(ConsistOf(hostcontrol.EventHostStopping))
		})

		It("logs transitions for the host", func() {
			origState := h.State

			Expect(hostcontrol.StopWithQueue(db, h, hbHeaders(), evCallback, u.ID)).To(BeNil())

			var count int
			err := db.QueryRowx(
				"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
				h.ID, host.StateStopping, origState).Scan(&count)
			Expect(err).To(BeNil())
			Expect(count).To(Equal(1))
		})
	})

	Describe("Terminate()", func() {
		var (
			hs *test.HostStack
			h  *host.Host
			c  *container.Container
			rt *test.FakeRoundTripper
		)

		BeforeEach(func() {
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:               "honeybadger",
					ProviderIdentifier: sql.NullString{"001", true},
					IPAddress:          sql.NullString{"10.0.0.1", true},
					RegionIdentifier:   sql.NullString{"us-east-1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())

			h, err = host.FindByIdentifier(db, hs.H.Slug)
			Expect(err).To(BeNil())

			c = &container.Container{
				CID:    "beefface",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Slug:   "beef-jerky",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		terminateHost := func() error {
			return db.WithTransaction(func(tx *sqlx.Tx) error {
				return hostcontrol.Terminate(tx, h, hbHeaders(), hs.H.UserID)
			})
		}

		Describe("when hostbuilder successfully deletes the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt = &test.FakeRoundTripper{
					Message: okResponse,
					Status:  http.StatusOK,
				}
				replaceTransport(hb, rt)
				hostcontrol.HostbuilderClient = hb
			})

			It("should send the UUID headers", func() {
				Expect(terminateHost()).To(BeNil())

				req := rt.Requests[0]
				Expect(req.Header.Get(constants.NitrousUUIDHeader)).ToNot(BeEmpty())
			})

			It("sends the correct payload to hostbuilder", func() {
				Expect(terminateHost()).To(BeNil())

				payload := &struct {
					ID        string `json:"id"`
					IPAddress string `json:"ip_address"`
					Region    string `json:"region_id"`
				}{}
				json.Unmarshal([]byte(rt.Requests[0].FormValue("config")), payload)

				Expect(payload.ID).To(Equal(hs.H.ProviderIdentifier.String), "invalid id")
				Expect(payload.IPAddress).To(Equal(hs.H.IPAddress.String), "invalid ip_address")
				Expect(payload.Region).To(Equal("us-east-1"), "invalid region")
			})

			It("deletes the host record", func() {
				Expect(terminateHost()).To(BeNil())

				_, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
			})

			It("deletes all container records belonging to the host", func() {
				c2, err := container.Find(db, hs.U, c.Slug)
				Expect(err).To(BeNil())
				Expect(c2).To(Equal(c))

				Expect(terminateHost()).To(BeNil())

				c2, err = container.Find(db, hs.U, c.Slug)
				Expect(err).To(MatchError(sql.ErrNoRows))
				Expect(c2).To(BeNil())
			})

			It("deletes keypairs associated to the host", func() {
				priv, _ := keypairs.GenerateKey()
				tx, _ := db.Beginx()
				keypairs.Create(tx, priv, "aes_key", hs.H.ID)
				tx.Commit()

				terminateHost()

				_, err := keypairs.Get(db, hs.H.ID)
				Expect(err).ToNot(BeNil(), "host's keypairs should be removed")
			})

			It("deletes certs associated to the host", func() {
				tx, _ := db.Beginx()
				crt := &cert.Cert{
					ServerCert: "servercert",
					ServerKey:  "serverkey",
				}
				Expect(crt.Insert(tx, hs.H.ID)).To(BeNil())
				tx.Commit()

				Expect(terminateHost()).To(BeNil())

				_, err := cert.Find(db, hs.H.ID)
				Expect(err).ToNot(BeNil(), "failed to remove host's certificates")
			})

			It("logs transitions for the host", func() {
				origState := hs.H.State

				terminateHost()

				var count int
				err := db.QueryRowx(
					"SELECT COUNT(*) FROM host_transitions WHERE host_id = $1 AND state = $2 AND previous_state = $3",
					hs.H.ID, host.StateTerminated, origState).Scan(&count)
				Expect(err).To(BeNil())
				Expect(count).To(Equal(1))
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &test.FakeRoundTripper{
					Message: errorResponse,
					Status:  http.StatusBadRequest,
				})
				hostcontrol.HostbuilderClient = hb
			})

			It("does not delete the host", func() {
				Expect(terminateHost()).ToNot(BeNil())

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
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

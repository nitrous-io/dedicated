package freighterd_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/freighterd"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"

	"github.com/nitrous-io/nitrous-common/msgverifier"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/freighterd")
}

var _ = Describe("controllers/freighterd", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = test.NewTestServer("", db, nil)
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /hosts/{slug}/freighterd_hook", func() {
		var (
			hs *test.HostStack
			u  *user.User
		)

		BeforeEach(func() {
			var err error
			hs, err = test.CreateHostStack(db, &test.HostStack{
				H: &host.Host{
					Slug:           "personal-141",
					IPAddress:      sql.NullString{"1.2.3.4", true},
					FreighterdPort: sql.NullInt64{64000, true},
					Size:           sql.NullString{"micro", true},
					State:          host.StateRunning,
				},
			})
			u = hs.U
			Expect(err).To(BeNil())
		})

		genMessage := func(secret string, msg *freighterd.Message) string {
			verifier := msgverifier.New(secret)
			signed, err := verifier.Generate(msg)
			Expect(err).To(BeNil())
			return signed
		}

		doRequest := func(slug, msg string) {
			var err error
			req, err = test.NewFormRequest("POST",
				fmt.Sprintf("/api/v0/hosts/%s/freighterd_hook", slug),
				&url.Values{"msg": []string{msg}})
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("with a verified 'started' message", func() {
			var msg string

			BeforeEach(func() {
				hs.H.State = host.StateUpdating
				hs.H.Update(db)

				m := &freighterd.Message{
					Event:             "started",
					DockerVersion:     "1.4.1-nitrous1",
					FreighterdVersion: "0.1.26-abcd",
				}
				msg = genMessage(hs.H.Secret, m)
			})

			It("accepts host ID in addition to slug", func() {
				doRequest(strconv.FormatInt(hs.H.ID, 10), msg)
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			It("responds with HTTP 200 and sets host state to running when host is in updating state", func() {
				doRequest(hs.H.Slug, msg)
				Expect(res.Code).To(Equal(http.StatusOK))
				h, err := host.FindByIdentifier(db, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("publishes a host software updated event", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug, msg)

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for host software updated event")
				case msg := <-ch:
					Expect(msg).NotTo(BeNil())
					ev, ok := msg.(*controllers.Event)
					Expect(ok).To(BeTrue())
					Expect(ev.Name).To(Equal(controllers.EventHostSoftwareUpdated))
				}
			})

			It("is 404 when host is not in updating state", func() {
				hs.H.State = host.StateRunning
				hs.H.Update(db)
				doRequest(hs.H.Slug, msg)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("with a verified 'ping' message", func() {
			var msg string

			BeforeEach(func() {
				hs.H.State = host.StateUpdating
				hs.H.Update(db)

				m := &freighterd.Message{Event: "ping"}
				msg = genMessage(hs.H.Secret, m)

				t := &token.Token{UserID: hs.H.UserID}
				err := t.Insert(db)
				Expect(err).To(BeNil())
			})

			It("accepts host ID in addition to slug", func() {
				doRequest(strconv.FormatInt(hs.H.ID, 10), msg)
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			It("responds with HTTP 200 and sets last_pinged_at to current time", func() {
				h, err := host.FindByIdentifier(db, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.LastPingedAt.Valid).To(BeFalse())

				doRequest(hs.H.Slug, msg)

				Expect(res.Code).To(Equal(http.StatusOK))
				h, err = host.FindByIdentifier(db, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.LastPingedAt.Valid).To(BeTrue())
				Expect(time.Now().Sub(h.LastPingedAt.Time)).To(BeNumerically("<=", 1*time.Second))
			})

			It("responds with HTTP 401 and does not update last_pinged_at if user is not logged in", func() {
				_, err := db.Exec(`DELETE FROM tokens WHERE user_id = $1`, hs.H.UserID)
				Expect(err).To(BeNil())
				h, err := host.FindByIdentifier(db, hs.H.Slug)
				Expect(err).To(BeNil())

				doRequest(hs.H.Slug, msg)

				Expect(res.Code).To(Equal(http.StatusUnauthorized))
				h, err = host.FindByIdentifier(db, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.LastPingedAt.Valid).To(BeFalse())
			})
		})

		Context("with a verified 'upgrade' message", func() {
			var msg string

			BeforeEach(func() {
				hs.H.State = host.StateUpdating
				hs.H.Update(db)

				m := &freighterd.Message{
					Event: "upgrade",
					Error: "failed to apt-get",
				}
				msg = genMessage(hs.H.Secret, m)
			})

			It("accepts host ID in addition to slug", func() {
				doRequest(strconv.FormatInt(hs.H.ID, 10), msg)
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			It("responds with HTTP 200 and sets host state to running when host is in updating state", func() {
				doRequest(hs.H.Slug, msg)
				Expect(res.Code).To(Equal(http.StatusOK))
				h, err := host.FindByIdentifier(db, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("publishes a host software update failed event", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug, msg)

				select {
				case <-time.After(50 * time.Millisecond):
					Fail("Timed out waiting for host software update failed event")
				case msg := <-ch:
					Expect(msg).NotTo(BeNil())
					ev, ok := msg.(*controllers.Event)
					Expect(ok).To(BeTrue())
					Expect(ev.Name).To(Equal(controllers.EventHostSoftwareUpdateFailed))
					e, ok := ev.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(e["error"]).To(Equal("failed to apt-get"))
				}
			})

			It("is 404 when host is not in updating state", func() {
				hs.H.State = host.StateRunning
				hs.H.Update(db)
				doRequest(hs.H.Slug, msg)
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		It("is 404 with a verified unrecognized event", func() {
			m := &freighterd.Message{
				Event: "nosuch",
			}
			msg := genMessage(hs.H.Secret, m)
			doRequest(hs.H.Slug, msg)
			Expect(res.Code).To(Equal(http.StatusNotFound))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("not_found"))
		})

		It("is 404 when message is unverified", func() {
			doRequest(hs.H.Slug, "Signed, John Hancock")
			Expect(res.Code).To(Equal(http.StatusNotFound))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("not_found"))
		})

		It("is 404 when host does not exist", func() {
			doRequest("nosuch", "")
			Expect(res.Code).To(Equal(http.StatusNotFound))
			errType, _ := test.ErrorSlug(res.Body)
			Expect(errType).To(Equal("host_not_found"))
		})
	})
})

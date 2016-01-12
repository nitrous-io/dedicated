package events_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/server"
	. "github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/events")
}

var _ = Describe("controllers/events", func() {
	var (
		db  *ndb.Database
		svr *server.Server

		u *user.User
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		svr = NewTestServer("", db, nil)

		u = &user.User{Username: "foo", Password: "123456", Email: "foo@example.com"}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t := &token.Token{UserID: u.ID, Token: "valid-token"}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Stream", func() {
		It("is a WebSocket endpoint that streams JSON-encoded events", func() {
			ts := httptest.NewServer(svr.Router())
			defer ts.Close()

			// Check that cross origin request works.
			header := http.Header{}
			header.Set("Origin", "http://www.example.com")

			params := &url.Values{}
			params.Set("token", "valid-token")
			wsURL, err := url.Parse(fmt.Sprintf("ws://%s/api/v0/events", ts.Listener.Addr().String()))
			Expect(err).To(BeNil())
			wsURL.RawQuery = params.Encode()
			dialer := websocket.Dialer{HandshakeTimeout: 100 * time.Millisecond}
			conn, dialResp, err := dialer.Dial(wsURL.String(), header)
			Expect(err).To(BeNil())
			defer conn.Close()
			Expect(dialResp.Header.Get("Sec-WebSocket-Accept")).NotTo(BeEmpty())

			type message struct {
				ID   int    `json:"id"`
				Text string `json:"text"`
			}
			topic := pubsub.Topic(fmt.Sprintf("user-%d", u.ID))
			svr.Events.Publish(message{ID: 1, Text: "moshi moshi"}, topic)
			svr.Events.Publish(message{ID: 2, Text: "siang ka pager?"}, topic)

			var out message
			errCh := make(chan error, 1)
			go func() {
				errCh <- conn.ReadJSON(&out)
			}()
			select {
			case err := <-errCh:
				Expect(err).To(BeNil())
				Expect(out).To(Equal(message{ID: 1, Text: "moshi moshi"}))
			case <-time.After(200 * time.Millisecond):
				Fail("Timeout reading JSON from WS connection.")
			}

			// Read 2nd message.
			go func() {
				errCh <- conn.ReadJSON(&out)
			}()
			select {
			case err := <-errCh:
				Expect(err).To(BeNil())
				Expect(out).To(Equal(message{ID: 2, Text: "siang ka pager?"}))
			case <-time.After(200 * time.Millisecond):
				Fail("Timeout reading JSON from WS connection.")
			}

			// Test that strings work too.
			svr.Events.Publish("hello hello", topic)

			var s string
			go func() {
				errCh <- conn.ReadJSON(&s)
			}()
			select {
			case err := <-errCh:
				Expect(err).To(BeNil())
				Expect(s).To(Equal("hello hello"))
			case <-time.After(200 * time.Millisecond):
				Fail("Timeout reading JSON from WS connection.")
			}
		})

		It("returns an error if not sent a valid bearer token", func() {
			ts := httptest.NewServer(svr.Router())
			defer ts.Close()

			wsEndpointURI := "ws://" + ts.Listener.Addr().String() + "/api/v0/events"
			_, _, err := websocket.DefaultDialer.Dial(wsEndpointURI, nil)
			Expect(err).To(Equal(websocket.ErrBadHandshake))
		})
	})
})

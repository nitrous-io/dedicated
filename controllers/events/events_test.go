package events_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/nitrous-io/nitrous-common/logger"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/pubsub"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
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

		svr = server.NewServer("", db, &logger.Logger{Level: logger.LogLevelQuiet})

		u = &user.User{Username: "foo", Password: "123456", Email: "foo@example.com"}
		err = u.Insert(db)
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

			// check that cross origin request works
			header := http.Header{}
			header.Set("Origin", "http://www.example.com")

			params := &url.Values{}
			params.Set("token", "valid-token")
			wsURL, err := url.Parse(fmt.Sprintf("ws://%s/api/v0/events", ts.Listener.Addr().String()))
			Expect(err).To(BeNil())
			wsURL.RawQuery = params.Encode()
			conn, dialResp, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
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
			err = conn.ReadJSON(&out)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(message{ID: 1, Text: "moshi moshi"}))
			err = conn.ReadJSON(&out)
			Expect(err).To(BeNil())
			Expect(out).To(Equal(message{ID: 2, Text: "siang ka pager?"}))

			svr.Events.Publish("hello hello", topic)

			var s string
			err = conn.ReadJSON(&s)
			Expect(err).To(BeNil())
			Expect(s).To(Equal("hello hello"))
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

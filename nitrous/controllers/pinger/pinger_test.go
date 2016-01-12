package pinger_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/server"
	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/pinger")
}

var _ = Describe("controllers/pinger", func() {
	var (
		db  *ndb.Database
		svr *server.Server

		u  *user.User
		hs *test.HostStack
	)

	BeforeEach(func() {
		var err error

		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		svr = test.NewTestServer("", db, nil)

		u = &user.User{Username: "foo", Password: "123456", Email: "foo@example.com"}
		err = test.WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t := &token.Token{UserID: u.ID, Token: "valid-token"}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		hs, err = test.CreateHostStack(db, &test.HostStack{U: u})
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	// Read acknowledgement message from WebSocket connection.
	var readAck = func(timestamp string, conn *websocket.Conn, ch chan<- struct{}, errCh chan<- error) {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}

			if string(msg) == "ok|"+timestamp {
				ch <- struct{}{}
				return
			}
		}
	}

	Describe("ForHost", func() {
		It("is a WebSocket endpoint updates a host's last_pinged_at when messages are sent to it", func() {
			ts := httptest.NewServer(svr.Router())
			defer ts.Close()

			// Check that cross origin request works.
			header := http.Header{}
			header.Set("Origin", "http://www.example.com")

			params := &url.Values{}
			params.Set("token", "valid-token")
			wsURL, err := url.Parse(fmt.Sprintf("ws://%s/api/v0/hosts/%d/activity",
				ts.Listener.Addr().String(), hs.H.ID))
			Expect(err).To(BeNil())
			wsURL.RawQuery = params.Encode()
			dialer := websocket.Dialer{HandshakeTimeout: 100 * time.Millisecond}
			conn, dialResp, err := dialer.Dial(wsURL.String(), header)
			Expect(err).To(BeNil())
			defer conn.Close()
			Expect(dialResp.Header.Get("Sec-WebSocket-Accept")).NotTo(BeEmpty())

			// Assert that last_pinged_at is initially null.
			Expect(hs.H.LastPingedAt.Time).To(BeZero())

			err = conn.WriteMessage(websocket.TextMessage, []byte("active|100"))
			Expect(err).To(BeNil())

			ch := make(chan struct{})
			errCh := make(chan error)

			go readAck("100", conn, ch, errCh)
			select {
			case <-ch:
				// Acknowledgement message received, continue.
			case err := <-errCh:
				Fail(fmt.Sprintf("error reading from WebSocket: %v", err))
			case <-time.After(100 * time.Millisecond):
				Fail("timed out waiting for message to be acknowledged")
			}

			h, err := host.FindByIdentifier(db, fmt.Sprintf("%d", hs.H.ID))
			Expect(err).To(BeNil())
			t1 := h.LastPingedAt.Time
			Expect(t1).To(BeTemporally("~", time.Now(), time.Second))

			// Send rubbish message.
			err = conn.WriteMessage(websocket.TextMessage, []byte("doesnt do anything"))
			Expect(err).To(BeNil())

			// Allow time for message to be read.
			<-time.After(100 * time.Millisecond)

			h, err = host.FindByIdentifier(db, fmt.Sprintf("%d", hs.H.ID))
			Expect(err).To(BeNil())
			t2 := h.LastPingedAt.Time
			Expect(t2).To(Equal(t1))

			// Send another proper message.
			err = conn.WriteMessage(websocket.TextMessage, []byte("active|101"))
			Expect(err).To(BeNil())

			go readAck("101", conn, ch, errCh)
			select {
			case <-ch:
				// Acknowledgement message received, continue.
			case err := <-errCh:
				Fail(fmt.Sprintf("error reading from WebSocket: %v", err))
			case <-time.After(100 * time.Millisecond):
				Fail("timed out waiting for message to be acknowledged")
			}

			h, err = host.FindByIdentifier(db, fmt.Sprintf("%d", hs.H.ID))
			Expect(err).To(BeNil())
			t3 := h.LastPingedAt.Time
			Expect(t3).To(BeTemporally("~", time.Now(), time.Second))
			Expect(t3).To(BeTemporally(">", t1))
		})

		It("also updates last_pinged_at if user is a collaborator of a container in the host", func() {
			collaborator := &user.User{
				Username: "shared",
				Email:    "shared@example.com",
				Password: "123456",
			}
			Expect(test.WrapInTransaction(db, collaborator.Insert)).To(BeNil())

			tok := &token.Token{
				UserID: collaborator.ID,
				Token:  "collaborator-token",
			}
			Expect(tok.Insert(db)).To(BeNil())

			cn := &container.Container{
				CID:    "abcde",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "test-container",
			}
			Expect(cn.Insert(db)).To(BeNil())

			_, err := db.Exec(`INSERT INTO container_collabs (container_id, user_id) VALUES ($1, $2);`,
				cn.ID, collaborator.ID)
			Expect(err).To(BeNil())

			ts := httptest.NewServer(svr.Router())
			defer ts.Close()

			params := &url.Values{}
			params.Set("token", tok.Token) // Use collaborator's token.
			wsURL, err := url.Parse(fmt.Sprintf("ws://%s/api/v0/hosts/%d/activity",
				ts.Listener.Addr().String(), hs.H.ID))
			Expect(err).To(BeNil())
			wsURL.RawQuery = params.Encode()
			dialer := websocket.Dialer{HandshakeTimeout: 100 * time.Millisecond}
			conn, dialResp, err := dialer.Dial(wsURL.String(), nil)
			Expect(err).To(BeNil())
			defer conn.Close()
			Expect(dialResp.Header.Get("Sec-WebSocket-Accept")).NotTo(BeEmpty())

			// Assert that last_pinged_at is initially null.
			Expect(hs.H.LastPingedAt.Time).To(BeZero())

			err = conn.WriteMessage(websocket.TextMessage, []byte("active|100"))
			Expect(err).To(BeNil())

			ch := make(chan struct{})
			errCh := make(chan error)

			go readAck("100", conn, ch, errCh)
			select {
			case <-ch:
				// Acknowledgement message received, continue.
			case err := <-errCh:
				Fail(fmt.Sprintf("error reading from WebSocket: %v", err))
			case <-time.After(100 * time.Millisecond):
				Fail("timed out waiting for message to be acknowledged")
			}

			h, err := host.FindByIdentifier(db, fmt.Sprintf("%d", hs.H.ID))
			Expect(err).To(BeNil())
			t1 := h.LastPingedAt.Time
			Expect(t1).To(BeTemporally("~", time.Now(), time.Second))
		})

		It("returns an error if not sent a valid bearer token", func() {
			ts := httptest.NewServer(svr.Router())
			defer ts.Close()

			uri := fmt.Sprintf("ws://%s/api/v0/hosts/%d/activity",
				ts.Listener.Addr().String(), hs.H.ID)
			_, _, err := websocket.DefaultDialer.Dial(uri, nil)
			Expect(err).To(Equal(websocket.ErrBadHandshake))
		})

		It("returns an error when host does not belong to user", func() {
			ts := httptest.NewServer(svr.Router())
			defer ts.Close()

			hs2, err := test.CreateHostStack(db, &test.HostStack{})
			Expect(err).To(BeNil())

			params := &url.Values{}
			params.Set("token", "valid-token")
			wsURL, err := url.Parse(fmt.Sprintf("ws://%s/api/v0/hosts/%d/activity",
				ts.Listener.Addr().String(), hs2.H.ID))
			Expect(err).To(BeNil())
			wsURL.RawQuery = params.Encode()

			_, _, err = websocket.DefaultDialer.Dial(wsURL.String(), nil)
			Expect(err).To(Equal(websocket.ErrBadHandshake))
		})

		It("responds with a HTTP 404 when the host has already been terminated", func() {
			_, err := db.Exec(`UPDATE hosts SET terminated_at=now() WHERE id = $1;`, hs.H.ID)
			Expect(err).To(BeNil())

			ts := httptest.NewServer(svr.Router())
			defer ts.Close()

			params := &url.Values{}
			params.Set("token", "valid-token")
			wsURL, err := url.Parse(fmt.Sprintf("ws://%s/api/v0/hosts/%d/activity",
				ts.Listener.Addr().String(), hs.H.ID))
			Expect(err).To(BeNil())
			wsURL.RawQuery = params.Encode()
			dialer := websocket.Dialer{HandshakeTimeout: 100 * time.Millisecond}
			_, dialResp, err := dialer.Dial(wsURL.String(), nil)
			Expect(err).To(Equal(websocket.ErrBadHandshake))
			Expect(dialResp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})
})

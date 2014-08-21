package events

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/pkg/wsconn"
)

const (
	// Max time allowed to write a message to the client.
	writeWait = 5 * time.Second

	// Max time allowed to read the next pong message from client.
	pongWait = 30 * time.Second

	// Pings are sent to the client with this period.
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
		// Do nothing. This is a workaround to prevent Upgrader from writing its
		// own plain/text HTTP response when Error is set to nil.
	},
}

func Stream(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if e, ok := err.(websocket.HandshakeError); ok {
			return httperror.New(http.StatusBadRequest, "invalid_request", fmt.Sprintf("WebSocket handshake error: %v", e))
		}
		return err
	}

	topic := env.User.Topic()
	ch := env.Events.Subscribe(topic)
	defer env.Events.Unsubscribe(ch, topic)

	conn := wsconn.New(ws, pongWait, pingPeriod, writeWait)
	defer conn.Close()

	quit := make(chan struct{})
	go func(quit <-chan struct{}) {
		// Write received events to WebSocket as JSON until told to quit.
		for {
			select {
			case msg := <-ch:
				if err := json.NewEncoder(conn).Encode(msg); err != nil {
					return
				}
			case <-quit:
				return
			}
		}
	}(quit)
	io.Copy(ioutil.Discard, conn) // Reads from the WebSocket are ignored.

	quit <- struct{}{}

	return nil
}

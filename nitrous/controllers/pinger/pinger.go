package pinger

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous-common/wsconn"
)

var msgRegex = regexp.MustCompile(`^active\|([0-9]+)$`)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
		// Do nothing. This is a workaround to prevent Upgrader from writing its
		// own plain/text HTTP response when Error is set to nil.
	},
	CheckOrigin: func(r *http.Request) bool {
		// allow any origin
		return true
	},
}

// ForHost is a WebSocket endpoint handler that accepts messages of the form
// "active|1443676945367" where "1443676945367" is a UNIX timestamp. It replies
// with "ok|1443676945367" (i.e. the same timestamp) as an acknowledgement.
func ForHost(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hostID := mux.Vars(r)["id"]
	h, err := host.Find(env.DB, env.User, hostID)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		h, err = host.FindShared(env.DB, env.User, hostID)
		if err != nil {
			if err == sql.ErrNoRows {
				return httperror.New(http.StatusNotFound, "not_found", "workspace not found")
			}
			return err
		}
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if e, ok := err.(websocket.HandshakeError); ok {
			return httperror.New(http.StatusBadRequest, "invalid_request", fmt.Sprintf("WebSocket handshake error: %v", e))
		}
		return err
	}

	// Use wsconn wrapper so we get server-side heartbeat support for free.
	conn := wsconn.NewConnectionHeartbeat(ws, true)
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			logrus.Infof("pinger.ForHost: error reading message from client, err: %v", err)
			break
		}

		submatches := msgRegex.FindSubmatch(msg)
		if submatches == nil {
			continue
		}

		timestamp := submatches[1]

		if err := h.Touch(env.DB); err != nil {
			return err
		}

		ack := fmt.Sprintf("ok|%s", timestamp)
		if _, err := conn.Write([]byte(ack)); err != nil {
			return err
		}
	}

	return nil
}

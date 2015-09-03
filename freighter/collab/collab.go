package collab

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/pkg/msgproto"
	"github.com/nitrous-io/ot.go/ot"
	"github.com/nitrous-io/ot.go/ot/operation"
	"github.com/nitrous-io/ot.go/ot/selection"
)

var (
	ConnectionTimeout = 300 * time.Second

	activeConnectionsLock sync.Mutex
	ActiveConnections     = map[string]int{}
)

func init() {
	ot.TextEncoding = ot.TextEncodingTypeUTF16
}

type MessageReaderWriter interface {
	msgproto.MessageReader
	io.Writer
}

func Handle(ws MessageReaderWriter, username, connID string) {
	var msgType int
	var d []byte
	var err error

	c := onConnect(ws, username, connID)

	for {
		msgType, d, err = msgproto.ReadAndDecodeMessage(ws)
		if err != nil {
			logrus.Errorf("wsCollab: msgproto.ReadAndDecodeMessage() failed with %s", err)
			break
		}
		switch msgType {
		case msgproto.CloseMessage:
			break
		case msgproto.DataMessage:
			if err = HandleCollabMessage(c, d); err != nil {
				logrus.Errorf("wsCollab: msgproto.HandleCollabMessage() failed with %s, message: %s", err, string(d))
				break
			}
		}
	}

	onDisconnect(c)
}

// {"namespace": "something", "event": "create", args: ["#include <stdio.h>\n....", "asfasf", 123123] }

func HandleCollabMessage(c *Connection, data []byte) error {
	var msg struct {
		FileName string        `json:"namespace"`
		Event    string        `json:"event"`
		Args     []interface{} `json:"args"`
	}
	err := json.Unmarshal(data, &msg)

	if err != nil {
		return err
	}

	switch msg.Event {
	case "join":
		doc, ok := msg.Args[0].(string)
		if !ok {
			return ErrInvalidArg
		}
		onJoin(c, msg.FileName, c.Username, doc)
	case "leave":
		onLeave(c, msg.FileName)
	case "rejoin":
		onRejoin(c, msg.FileName)
	case "op":
		onOp(c, msg.FileName, msg.Args)
	case "sel":
		onSel(c, msg.FileName, msg.Args)
	case "active":
		onActive(c)
	}

	return nil
}

func onConnect(ws MessageReaderWriter, username, connID string) *Connection {
	c := NewConnection(ws, username, connID)
	// if connection id is given, this is a reconnect
	if connID != "" {
		UpdateConnection(c)
	}
	activeConnectionsLock.Lock()
	ActiveConnections[c.ID]++
	activeConnectionsLock.Unlock()
	c.Send("", "connected", map[string]interface{}{
		"conn_id": c.ID,
	})
	return c
}

func onDisconnect(c *Connection) {
	activeConnectionsLock.Lock()
	if ActiveConnections[c.ID] > 0 {
		ActiveConnections[c.ID]--
	}
	activeConnectionsLock.Unlock()

	time.AfterFunc(ConnectionTimeout, func() {
		activeConnectionsLock.Lock()
		defer activeConnectionsLock.Unlock()
		if ActiveConnections[c.ID] > 0 {
			return
		}
		for _, s := range Sessions {
			if _, ok := s.Connections[c.ID]; ok {
				c.Broadcast(s, "disconnected", map[string]string{
					"conn_id": c.ID,
				})
			}
		}
		UnregisterConnectionGlobally(c)
	})
}

func onJoin(c *Connection, fileName, username, doc string) {
	s := NewSession(fileName, doc)

	c.Send(fileName, "doc", map[string]interface{}{
		"document": s.Document,
		"revision": len(s.Session.Operations),
		"clients":  s.Session.Clients,
	})

	s.RegisterConnection(c)

	c.Send(fileName, "registered", map[string]string{
		"conn_id": c.ID,
	})

	broadcastAll(s, c, "joined", map[string]string{
		"conn_id":  c.ID,
		"username": c.Username,
	})
}

func onLeave(c *Connection, fileName string) {
	s := GetSession(fileName)
	if s != nil {
		s.UnregisterConnection(c)
	}

	c.Send(fileName, "unregistered", nil)
	broadcastAll(s, c, "left", map[string]string{
		"conn_id":  c.ID,
		"username": c.Username,
	})
}

func broadcastAll(thisSession *Session, c *Connection, event string, data interface{}) {
	connMap := map[*Connection]bool{}
	for _, s := range Sessions {
		for _, conn := range s.Connections {
			if conn == c {
				continue
			}

			if ok, _ := connMap[conn]; !ok {
				connMap[conn] = true
				conn.Send(thisSession.ID, event, data)
			}
		}
	}
}

func onRejoin(c *Connection, fileName string) {
	s := GetSession(fileName)
	if s != nil && s.Connections[c.ID] == c {
		c.Send(fileName, "rejoined", nil)
	} else {
		c.Send(fileName, "rejoin_failed", nil)
	}
}

func onActive(c *Connection) {
	sessions := map[string]interface{}{}
	for _, s := range Sessions {
		sessions[s.ID] = map[string]interface{}{
			"document": s.Document,
			"revision": len(s.Session.Operations),
			"clients":  s.Session.Clients,
		}
	}
	c.Send("", "active", map[string]interface{}{
		"sessions": sessions,
	})
}

func onOp(c *Connection, fileName string, data []interface{}) {
	s := GetSession(fileName)
	if s == nil {
		return
	}

	// data: [revision, ops, selection?]
	if len(data) < 2 {
		return
	}
	// revision
	revf, ok := data[0].(float64)
	rev := int(revf)
	if !ok {
		return
	}
	// ops
	ops, ok := data[1].([]interface{})
	if !ok {
		return
	}
	top, err := operation.Unmarshal(ops)
	if err != nil {
		return
	}
	// selection (optional)
	if len(data) >= 3 {
		selm, ok := data[2].(map[string]interface{})
		if ok {
			sel, err := selection.Unmarshal(selm)
			if err == nil {
				top.Meta = sel
			}
		}
	}

	s.Lock.Lock()
	defer s.Lock.Unlock()

	top2, err := s.AddOperation(rev, top)
	if err != nil {
		return
	}

	err = c.Send(fileName, "ok", nil)
	if err != nil {
		return
	}

	if sel, ok := top2.Meta.(*selection.Selection); ok {
		s.SetSelection(c.ID, sel)
		c.Broadcast(s, "op", []interface{}{c.ID, top2.Marshal(), sel.Marshal()})
	} else {
		c.Broadcast(s, "op", []interface{}{c.ID, top2.Marshal()})
	}
}

func onSel(c *Connection, fileName string, data []interface{}) {
	// data: [selection, revision?]
	if len(data) == 0 {
		return
	}

	s := GetSession(fileName)
	if s == nil || data == nil {
		return
	}

	// selection
	var sel *selection.Selection
	var err error
	selm, ok := data[0].(map[string]interface{})
	if ok {
		sel, err = selection.Unmarshal(selm)
		if err != nil {
			return
		}
	} else {
		return
	}

	// revision
	var rev int
	if len(data) >= 2 {
		if revf, ok := data[1].(float64); ok {
			rev = int(revf)
		}
	}

	s.Lock.Lock()
	s.SetSelection(c.ID, sel)
	c.Broadcast(s, "sel", []interface{}{c.ID, selm, rev})
	s.Lock.Unlock()
}

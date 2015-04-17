package collab

import (
	"encoding/json"
	"io"
	"log"

	"github.com/nitrous-io/dedicated/pkg/msgproto"
	"github.com/nitrous-io/ot.go/ot"
	"github.com/nitrous-io/ot.go/ot/operation"
	"github.com/nitrous-io/ot.go/ot/selection"
)

func init() {
	ot.TextEncoding = ot.TextEncodingTypeUTF16
}

type MessageReaderWriter interface {
	msgproto.MessageReader
	io.Writer
}

func Handle(username string, ws MessageReaderWriter) {
	var msgType int
	var d []byte
	var err error

	c := NewConnection(username, ws)

	for {
		msgType, d, err = msgproto.ReadAndDecodeMessage(ws)
		if err != nil {
			log.Printf("wsCollab: msgproto.ReadAndDecodeMessage() failed with %s", err)
			break
		}
		switch msgType {
		case msgproto.CloseMessage:
			break
		case msgproto.DataMessage:
			if err = HandleCollabMessage(c, d); err != nil {
				log.Printf("wsCollab: msgproto.HandleCollabMessage() failed with %s, message: %s", err, string(d))
				break
			}
		}
	}

	for _, s := range Sessions {
		if _, ok := s.Connections[c]; ok {
			c.Broadcast(s, "left", map[string]string{
				"client_id": c.ID,
			})
		}
	}

	UnregisterConnectionGlobally(c)
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
		join(c, msg.FileName, c.Username, doc)
	case "leave":
		leave(c, msg.FileName)
	case "op":
		op(c, msg.FileName, msg.Args)
	case "sel":
		data, ok := msg.Args[0].(map[string]interface{})
		if !ok {
			return ErrInvalidArg
		}
		sel(c, msg.FileName, data)
	}

	return nil
}

func join(c *Connection, fileName, username, doc string) {
	s := NewSession(fileName, doc)

	c.Send(s, "doc", map[string]interface{}{
		"document": s.Document,
		"revision": len(s.Session.Operations),
		"clients":  s.Session.Clients,
	})

	s.RegisterConnection(c)

	c.Send(s, "registered", map[string]string{
		"client_id": c.ID,
	})

	c.Broadcast(s, "joined", map[string]string{
		"client_id": c.ID,
		"username":  c.Username,
	})
}

func leave(c *Connection, fileName string) {
	s := GetSession(fileName)
	if s != nil {
		s.UnregisterConnection(c)
	}

	c.Send(s, "unregistered", nil)
	c.Broadcast(s, "left", map[string]string{
		"client_id": c.ID,
	})
}

func op(c *Connection, fileName string, data []interface{}) {
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
		if !ok {
			return
		}
		sel, err := selection.Unmarshal(selm)
		if err != nil {
			return
		}
		top.Meta = sel
	}

	s.Lock.Lock()
	defer s.Lock.Unlock()

	top2, err := s.AddOperation(rev, top)
	if err != nil {
		return
	}

	err = c.Send(s, "ok", nil)
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

func sel(c *Connection, fileName string, data map[string]interface{}) {
	s := GetSession(fileName)
	if s == nil {
		return
	}

	sel, err := selection.Unmarshal(data)
	if err != nil {
		return
	}

	s.Lock.Lock()
	defer s.Lock.Unlock()

	s.SetSelection(c.ID, sel)
	c.Broadcast(s, "sel", []interface{}{c.ID, sel.Marshal()})
}

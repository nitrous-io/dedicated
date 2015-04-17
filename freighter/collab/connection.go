package collab

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
)

var (
	ErrInvalidArg = errors.New("collab: invalid argument given")

	nextConnID     = int64(0)
	nextConnIDLock sync.Mutex
)

type Connection struct {
	ID       string
	Username string
	Conn     io.Writer
}

func NewConnection(username string, conn io.Writer) *Connection {
	nextConnIDLock.Lock()
	c := &Connection{
		ID:       strconv.FormatInt(nextConnID, 36),
		Username: username,
		Conn:     conn,
	}
	nextConnID++
	nextConnIDLock.Unlock()
	return c
}

func (c *Connection) Send(s *Session, event string, data interface{}) error {
	if c.Conn != nil {
		e := []interface{}{s.ID, event, data}
		b := &bytes.Buffer{}
		_, err := b.WriteString("d|")
		if err != nil {
			return err
		}
		err = json.NewEncoder(b).Encode(e)
		if err != nil {
			return err
		}
		// data should be written with a single Write call to make sure only one
		// websocket message (frame) is sent
		_, err = c.Conn.Write(b.Bytes())
		return err
	}
	return nil
}

func (c *Connection) Broadcast(s *Session, event string, data interface{}) {
	for conn, _ := range s.Connections {
		if conn != c {
			conn.Send(s, event, data)
		}
	}
}

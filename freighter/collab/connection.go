package collab

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
)

var (
	ErrInvalidArg = errors.New("collab: invalid argument given")

	nextConnID = int64(0)
)

type Connection struct {
	ID         string
	Username   string
	Conn       io.Writer
	bufferLock sync.Mutex
	Buffer     [][]byte
}

func NewConnection(conn io.Writer, username, id string) *Connection {
	if id == "" {
		id = strconv.FormatInt(atomic.AddInt64(&nextConnID, 1), 36)
	}
	c := &Connection{
		ID:       id,
		Username: username,
		Conn:     conn,
		Buffer:   [][]byte{},
	}
	return c
}

func (c *Connection) Send(namespace, event string, data interface{}) error {
	if c.Conn != nil {
		e := []interface{}{namespace, event, data}
		b := &bytes.Buffer{}
		_, err := b.WriteString("d|")
		if err != nil {
			return err
		}
		err = json.NewEncoder(b).Encode(e)
		if err != nil {
			return err
		}

		// append to buffer
		c.bufferLock.Lock()
		c.Buffer = append(c.Buffer, b.Bytes())

		for _, bytes := range c.Buffer {
			_, err = c.Conn.Write(bytes)
			if err != nil {
				break
			}
			c.Buffer = c.Buffer[1:]
		}
		c.bufferLock.Unlock()

		// data should be written with a single Write call to make sure only one
		// websocket message (frame) is sent
		return err
	}
	return nil
}

func (c *Connection) Broadcast(s *Session, event string, data interface{}) {
	if s == nil {
		return
	}
	for _, conn := range s.Connections {
		if conn != c {
			conn.Send(s.ID, event, data)
		}
	}
}

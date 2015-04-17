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
	ID         string
	Username   string
	Conn       io.Writer
	Buffer     [][]byte
	bufferLock sync.Mutex
}

func NewConnection(conn io.Writer, username, id string) *Connection {
	if id == "" {
		nextConnIDLock.Lock()
		id = strconv.FormatInt(nextConnID, 36)
		nextConnID++
		nextConnIDLock.Unlock()
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

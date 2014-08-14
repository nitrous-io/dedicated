package wsconn

import (
	"io"
	"time"

	"github.com/gorilla/websocket"
)

type Conn struct {
	*websocket.Conn
	reader     io.Reader
	writeWait  time.Duration
	pongWait   time.Duration
	pingPeriod time.Duration
}

func New(ws *websocket.Conn, pongWait, pingPeriod, writeWait time.Duration) *Conn {
	c := &Conn{Conn: ws,
		pongWait:   pongWait,
		pingPeriod: pingPeriod,
		writeWait:  writeWait,
	}
	c.pingpong()
	return c
}

func (c *Conn) Write(b []byte) (int, error) {
	if c.writeWait != 0 {
		c.SetWriteDeadline(time.Now().Add(c.writeWait))
	}
	if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *Conn) Read(b []byte) (int, error) {
	if c.reader == nil {
		t, r, err := c.NextReader()
		if err != nil {
			return 0, err
		}
		if t != websocket.TextMessage {
			return 0, nil
		}
		c.reader = r
	}
	n, err := c.reader.Read(b)
	if err != nil {
		if err == io.EOF {
			c.reader = nil
		}
		return n, err
	}
	return n, nil
}

func (c *Conn) Close() error {
	c.SetWriteDeadline(time.Now().Add(c.writeWait))
	c.WriteMessage(websocket.CloseMessage, []byte{})
	return c.Conn.Close()
}

func (c *Conn) pingpong() {
	if c.pongWait != 0 {
		c.SetReadDeadline(time.Now().Add(c.pongWait))
		c.SetPongHandler(func(string) error {
			c.SetReadDeadline(time.Now().Add(c.pongWait))
			return nil
		})
	}

	if c.pingPeriod != 0 {
		pingTicker := time.NewTicker(c.pingPeriod)
		go func() {
			defer pingTicker.Stop()
			for _ = range pingTicker.C {
				if c.writeWait != 0 {
					c.SetWriteDeadline(time.Now().Add(c.writeWait))
				}
				if err := c.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					return
				}
			}
		}()
	}
}

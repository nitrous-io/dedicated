/*
server <--> client

The message is json-encoded with type

--> m|{type: "fs.list", cid: callbackID, args: ["path"]} // list a directory
<-- m|{type: "fs.list", cid: callbackID, args: ["dir1", "dir2]} // reply a message with some response
<-- c|we will be back // close message
--> h // heartbeat message

NOTE: this is pretty similar to the shell protocol. Maybe we could refactor
something here.
*/
package msgproto

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
)

var (
	dataMsg      = []byte("d|")
	closeMsg     = []byte("c|")
	heartbeatMsg = []byte("h")

	InvalidMessage = errors.New("invalid message")
)

type Message fmt.Stringer

// InvalidMsg is returned whenever the client sends a message with a invalid
// protocol that we dont recognize
type InvalidMsg struct{ msg string }

func (m *InvalidMsg) String() string { return m.msg }

// CloseMsg is the close msg that client sends before closing the connection
type CloseMsg struct{ msg string }

func (m *CloseMsg) String() string { return fmt.Sprintf("close: %s", m.msg) }

type websocketReader interface {
	NextReader() (messageType int, r io.Reader, err error)
}

type Reader struct {
	socket websocketReader

	// cached reader, which is reset whenever the messages is EOF
	reader io.Reader
	// for out of band input such as heartbeat / resize / close
	msgChan chan Message

	// message type for the protocol
	MessageType int
}

func NewReader(ws websocketReader, ch chan Message, t int) io.Reader {
	return &Reader{socket: ws, msgChan: ch, MessageType: t}
}

func (s *Reader) Read(p []byte) (int, error) {
	var newMessage bool
	// if we didnt have a reader, this is a new message and we should get a
	// reader for it
	if s.reader == nil {
		t, r, err := s.socket.NextReader()
		if err != nil {
			return 0, err
		}
		if t != s.MessageType {
			log.Printf("wsconn: ignored unknown message type. Got %d, wanted %d", t, s.MessageType)
			return 0, nil
		}
		s.reader = r
		newMessage = true
	}
	l := len(p)
	buf := make([]byte, l)
	n, err := s.reader.Read(buf)
	if err != nil {
		// When we have the frame EOF, treat it as no-op so we can receive the
		// next frame normally
		if err == io.EOF {
			s.reader = nil
			err = nil
		}
		return 0, err
	}
	// we ignore the heartbeat msg from the client
	if newMessage && n == 1 && bytes.Equal(buf[0:1], heartbeatMsg) {
		return 0, nil
	}
	// if this is a new message, we takes out the protocol prefix
	if newMessage && n > 2 {
		// detect client input, copy everything back to p without the prefix
		if bytes.Equal(buf[0:2], dataMsg) {
			i := copy(p, buf[2:n])
			return i, err
		}
		// detect close msg
		if bytes.Equal(buf[0:2], closeMsg) {
			msg := &CloseMsg{string(buf[2:n])}
			s.msgChan <- msg
			return 0, nil
		}
		// ignore the invalid message
	}
	// if this is not a new message, this is a fragment message and we just
	// copy it over without caring about the prefix
	if !newMessage {
		copy(p, buf)
		return n, err
	}
	return 0, nil
}

func WriteMsg(p []byte, w io.Writer) (int, error) {
	l := len(p)
	buf := make([]byte, l+len(dataMsg))
	copy(buf[0:2], dataMsg)
	copy(buf[2:], p)
	return w.Write(buf)
}

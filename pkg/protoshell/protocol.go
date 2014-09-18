// server <--> client
// --> d|action@foobar $ # output from server
// <-- d|whoami          # input from client
// --> d|action          # output from server
// <-- r|80 25           # resize from client
// <-- c|good bye        # close msg
//
// NOT IMPLEMENTED
// <-- h                  # heartbeat from client
//
// FIXME: look at freighter/pkg/shell and see if we can somehow merge/reuse the
// readers/writers
package protoshell

import (
	"bytes"
	"io"
)

var (
	dataMsg      = []byte("d|")
	resizeMsg    = []byte("r|")
	closeMsg     = []byte("c|")
	heartbeatMsg = []byte("h")
)

// ShellWriter handles server write which outputs d|server_output
type ShellWriter struct {
	writer io.Writer
}

func NewShellWriter(w io.Writer) io.Writer {
	return &ShellWriter{writer: w}
}

func (s *ShellWriter) Write(p []byte) (int, error) {
	l := len(p)
	buf := make([]byte, l+len(dataMsg))
	copy(buf[0:2], dataMsg)
	copy(buf[2:], p)
	_, err := s.writer.Write(buf)
	return l, err
}

type websocketReader interface {
	NextReader() (messageType int, r io.Reader, err error)
}

// RemoteReader acts like an io.Reader, which reads the messages from websocket
// and returns only output from the data messages, ignore everything else
type ShellReader struct {
	socket websocketReader

	// cached reader, which is reset whenever the messages is EOF
	reader io.Reader

	// message type for the protocol
	MessageType int
}

func NewShellReader(ws websocketReader, t int) io.Reader {
	return &ShellReader{socket: ws, MessageType: t}
}

func (s *ShellReader) Read(p []byte) (int, error) {
	var newMessage bool
	// if we didnt have a reader, this is a new message and we should get a
	// reader for it
	if s.reader == nil {
		t, r, err := s.socket.NextReader()
		if err != nil {
			return 0, err
		}
		if t != s.MessageType {
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
		} else {
			// ignore other control messages
			return 0, nil
		}
	}
	// if this is not a new message, this is a fragment message and we just
	// copy it over without caring about the prefix
	if !newMessage {
		copy(p, buf)
		return n, err
	}
	return 0, nil
}

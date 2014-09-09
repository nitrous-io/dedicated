// this handles our websocket protocol
//
// d|action@foobar    # output from server
// p|whoami           # input from client
// d|action           # output from server
// w|80 25            # resize from client
// c|server went away # close msg
//
// NOT IMPLEMENTED
// h                  # heartbeat from client
package shell

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

var (
	serverOutputMsg = []byte("d|")
	clientInputMsg  = []byte("p|")
	resizeMsg       = []byte("w|")
	closeMsg        = []byte("c|")
	heartbeatMsg    = []byte("h")

	InvalidMessage = errors.New("invalid message")
)

type Message fmt.Stringer

// InvalidMsg is returned whenever the client sends a message with a invalid
// protocol that we dont recognize
type InvalidMsg struct {
	msg string
}

func (m *InvalidMsg) String() string { return m.msg }

// ResizeMsg implements Stringer interfaces, and contains the desirable width and
// height. This will be write to a channel that handles non input/output message
//
// Refers to ShellReader for more information
type ResizeMsg struct {
	width  int
	height int
}

func (m *ResizeMsg) String() string {
	return fmt.Sprintf("resize: w %d, h %d", m.width, m.height)
}

func NewResizeMsg(msg []byte) fmt.Stringer {
	in := bytes.NewBuffer(msg)
	var w, h int
	i, err := fmt.Fscanf(in, "%d %d", &w, &h)
	if err != nil || i != 2 {
		return &InvalidMsg{fmt.Sprintf("invalid resize msg %s", string(msg))}
	}
	return &ResizeMsg{width: w, height: h}
}

// CloseMsg represents the close msg that client sends before closing the
// connection
type CloseMsg struct {
	msg string
}

func (m *CloseMsg) String() string {
	return fmt.Sprintf("close: %s", m.msg)
}

// ShellReader handles client input which allows type:
// - p|
// - w|
// - c|
// - h
type ShellReader struct {
	reader io.Reader
	// for out of band input such as heartbeat / resize / close
	msgChan chan Message
}

func NewShellReader(r io.Reader, ch chan Message) io.Reader {
	return &ShellReader{reader: r, msgChan: ch}
}

func (s *ShellReader) Read(p []byte) (int, error) {
	l := len(p)
	buf := make([]byte, l)
	n, err := s.reader.Read(buf)
	// if we had non-eof error, fail early
	if err != nil && err != io.EOF {
		return 0, fmt.Errorf("ShellReader: failed to read with err %s", err)
	}
	if n >= 2 {
		// detect client input, copy everything back to p without the prefix
		if bytes.Equal(buf[0:2], clientInputMsg) {
			i := copy(p, buf[2:n])
			return i, err
		}
		// detect resize msg
		if bytes.Equal(buf[0:2], resizeMsg) {
			msg := NewResizeMsg(buf[2:n])
			s.msgChan <- msg
			return 0, nil
		}
		// detect close msg
		if bytes.Equal(buf[0:2], closeMsg) {
			msg := &CloseMsg{string(buf[2:n])}
			s.msgChan <- msg
			return 0, nil
		}
	}
	return 0, err
}

// ShellWriter handles server write which outputs d|server_output
type ShellWriter struct {
	writer io.Writer
}

func NewShellWriter(w io.Writer) io.Writer {
	return &ShellWriter{writer: w}
}

func (s *ShellWriter) Write(p []byte) (int, error) {
	l := len(p)
	buf := make([]byte, l+len(serverOutputMsg))
	copy(buf[0:2], serverOutputMsg)
	copy(buf[2:], p)
	_, err := s.writer.Write(buf)
	return l, err
}

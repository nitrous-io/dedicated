// this handles our websocket protocol
// server <--> client
// --> d|nitrous@foobar $ # output from server
// <-- d|whoami           # input from client
// --> d|nitrous          # output from server
// <-- r|80 25            # resize from client
// <-- c|good bye         # close msg from client
// --> c|                 # close msg from server
//
// NOT IMPLEMENTED
// <-- h                  # heartbeat from client
package shell

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
)

var (
	dataMsg      = []byte("d|")
	resizeMsg    = []byte("r|")
	closeMsg     = []byte("c|")
	heartbeatMsg = []byte("h")

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
	Width  int
	Height int
}

func (m *ResizeMsg) String() string {
	return fmt.Sprintf("resize: w %d, h %d", m.Width, m.Height)
}

func NewResizeMsg(msg []byte) fmt.Stringer {
	in := bytes.NewBuffer(msg)
	var w, h int
	i, err := fmt.Fscanf(in, "%d %d", &w, &h)
	if err != nil || i != 2 {
		return &InvalidMsg{fmt.Sprintf("invalid resize msg %s", string(msg))}
	}
	return &ResizeMsg{Width: w, Height: h}
}

// CloseMsg represents the close msg that client sends before closing the
// connection
type CloseMsg struct {
	msg string
}

func (m *CloseMsg) String() string {
	return fmt.Sprintf("close: %s", m.msg)
}

type websocketReader interface {
	NextReader() (messageType int, r io.Reader, err error)
}

type EncoderDecoder interface {
	Encode(data []byte) []byte
	Decode(data []byte) ([]byte, error)
}

func NoopEncode(data []byte) []byte {
	return data
}

func NoopDecode(data []byte) ([]byte, error) {
	return data, nil
}

// Implementation copy from encoding/base64/base64.go
func Base64Encode(decoded []byte) []byte {
	result := make([]byte, base64.StdEncoding.EncodedLen(len(decoded)))
	base64.StdEncoding.Encode(result, decoded)
	return result
}

func Base64Decode(encoded []byte) ([]byte, error) {
	result := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	n, err := base64.StdEncoding.Decode(result, encoded)
	return result[:n], err
}

type EncoderImpl struct {
	EncoderFunc func(data []byte) []byte          `json:"-"` // json annotation to
	DecoderFunc func(data []byte) ([]byte, error) `json:"-"` // prevent it being serialized
}

func (impl *EncoderImpl) Encode(data []byte) []byte {
	return impl.EncoderFunc(data)
}

func (impl *EncoderImpl) Decode(data []byte) ([]byte, error) {
	return impl.DecoderFunc(data)
}

var PlainEncoder = &EncoderImpl{
	NoopEncode,
	NoopDecode,
}

var Base64Encoder = &EncoderImpl{
	Base64Encode,
	Base64Decode,
}

// ShellReader handles client input which allows type:
// - d|
// - r|
// - c|
// - h
type ShellReader struct {
	socket websocketReader

	// cached reader, which is reset whenever the messages is EOF
	reader io.Reader
	// for out of band input such as heartbeat / resize / close
	msgChan chan Message

	// message type for the protocol
	MessageType int

	Encoder EncoderDecoder
}

func NewShellReader(ws websocketReader, ch chan Message, t int, encoder EncoderDecoder) io.Reader {
	return &ShellReader{socket: ws, msgChan: ch, MessageType: t, Encoder: encoder}
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
			encoded := buf[2:n]
			decoded, err := s.Encoder.Decode(encoded)
			if err != nil {
				return 0, err
			}

			i := copy(p, decoded)
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
	// if this is not a new message, this is a fragment message and we just
	// copy it over without caring about the prefix
	if !newMessage {
		copy(p, buf)
		return n, err
	}
	return 0, nil
}

// ShellWriter takes data written to it and writes it as a data message of the
// form "d|server_output". It writes server output to be sent to the client.
type ShellWriter struct {
	writer  io.Writer
	Encoder EncoderDecoder
}

// NewShellWriter creates a new ShellWriter that satisfies io.WriteCloser.
// It is the caller's responsibility to call Close on the WriteCloser when done.
func NewShellWriter(w io.Writer, encoder EncoderDecoder) *ShellWriter {
	return &ShellWriter{writer: w, Encoder: encoder}
}

func (s *ShellWriter) Write(p []byte) (int, error) {
	encoded := s.Encoder.Encode(p)

	messageLength := len(encoded)
	buf := make([]byte, messageLength+len(dataMsg))
	copy(buf[0:2], dataMsg)
	copy(buf[2:], encoded)
	_, err := s.writer.Write(buf)
	return len(p), err
}

// Close sends a close message to the client, but does not close the underlying
// io.Writer.
func (s *ShellWriter) Close() error {
	_, err := s.writer.Write(closeMsg)
	return err
}

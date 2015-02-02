package shell

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type wsReader struct {
	io.Reader
}

func (ws wsReader) NextReader() (int, io.Reader, error) {
	return websocket.BinaryMessage, ws.Reader, nil
}

func TestNewShellReader(t *testing.T) {
	testCases := []struct {
		msg         string
		expected    string
		expectedMsg Message
	}{
		{`d|whoami`, `whoami`, nil},
		{`d|ls`, `ls`, nil},
		{`r|80 20`, ``, &ResizeMsg{Width: 80, Height: 20}},
		{`c|good bye`, ``, &CloseMsg{msg: `good bye`}},
	}

	for _, tc := range testCases {
		in := bytes.NewBufferString(tc.msg)
		ch := make(chan Message, 1)
		s := NewShellReader(wsReader{Reader: in}, ch, websocket.BinaryMessage, PlainEncoder)

		buf := make([]byte, 100)
		n, _ := s.Read(buf)
		if n != len(tc.expected) {
			t.Errorf("expect to read %d bytes but got %d bytes", len(tc.msg), n)
		}

		if tc.expectedMsg != nil {
			select {
			case msg := <-ch:
				if !reflect.DeepEqual(msg, tc.expectedMsg) {
					t.Errorf("expect msg %v but got %v", tc.expectedMsg, msg)
				}
			case <-time.After(10 * time.Millisecond):
				t.Errorf("timed out while waiting for %v", tc.expectedMsg)
			}
		}

		out := string(buf[:n])
		if out != tc.expected {
			t.Errorf("expect output %s but got %s", tc.expected, out)
		}
	}
}

func TestReadFragmentedMessages(t *testing.T) {
	testCases := []struct {
		encodedMessages []string
		decodedMessage  string
		encoder         EncoderDecoder
	}{
		{
			encodedMessages: []string{
				"d|hello-",
				"fragmented-message",
			},
			decodedMessage: "hello-fragmented-message",
			encoder:        PlainEncoder,
		},
		/* TODO: Base64 not handling fragmented message correctly. :(
		{
			encodedMessages: []string{
				"d|aGVsbG8tZnJhZ21l",
				"bnRlZC1tZXNzYWdl",
			},
			decodedMessage: "hello-fragmented-message",
			encoder:        Base64Encoder,
		},
		*/
	}

	for _, tc := range testCases {
		r, w := io.Pipe()
		s := NewShellReader(wsReader{Reader: r}, make(chan Message), websocket.BinaryMessage, tc.encoder)

		go func() {
			for _, fragment := range tc.encodedMessages {
				w.Write([]byte(fragment))
			}
			w.CloseWithError(io.ErrClosedPipe)
		}()

		buf := bytes.NewBuffer(nil)
		_, err := io.Copy(buf, s)
		if err != io.ErrClosedPipe {
			t.Errorf("expect err %v but got %v", io.ErrClosedPipe, err)
		}
		expected := tc.decodedMessage
		if buf.String() != expected {
			t.Errorf("expect %s but got %v", expected, buf.String())
		}
	}
}

func TestNewShellWriter(t *testing.T) {
	testCases := []struct {
		msg      string
		expected string
		encoder  EncoderDecoder
	}{
		{`nitrous@foobar`, `d|nitrous@foobar`, PlainEncoder},
		{`nitrous@foobar`, `d|bml0cm91c0Bmb29iYXI=`, Base64Encoder},
	}

	for _, tc := range testCases {
		w := bytes.NewBuffer(nil)
		s := NewShellWriter(w, tc.encoder)

		r := bytes.NewBufferString(tc.msg)
		io.Copy(s, r)

		if w.String() != tc.expected {
			t.Errorf("expect %s to be written but got %s", tc.expected, w.String())
		}
	}
}

func TestShellWriterClose(t *testing.T) {
	w := bytes.NewBuffer(nil)
	s := NewShellWriter(w, PlainEncoder)

	s.Close()

	if w.String() != `c|` {
		t.Errorf("expect close message to be written but got %s", w.String())
	}
}

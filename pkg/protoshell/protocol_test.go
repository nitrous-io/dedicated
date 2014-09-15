package protoshell

import (
	"bytes"
	"io"
	"testing"

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
		msg      string
		expected string
	}{
		{`d|whoami`, `whoami`},
		{`d|ls`, `ls`},
		{`r|80 20`, ``},
		{`c|good bye`, ``},
		{`h`, ``},
	}

	for _, tc := range testCases {
		in := bytes.NewBufferString(tc.msg)
		s := NewShellReader(wsReader{Reader: in}, websocket.BinaryMessage)

		buf := make([]byte, 100)
		n, _ := s.Read(buf)
		if n != len(tc.expected) {
			t.Errorf("expect to read %d bytes but got %d bytes", len(tc.msg), n)
		}

		out := string(buf[:n])
		if out != tc.expected {
			t.Errorf("expect output %s but got %s", tc.expected, out)
		}
	}
}

func TestReadFragmentedMessages(t *testing.T) {
	r, w := io.Pipe()
	s := NewShellReader(wsReader{Reader: r}, websocket.BinaryMessage)

	go func() {
		w.Write([]byte(`d|hello-`))
		w.Write([]byte(`fragmented-message`))
		w.CloseWithError(io.ErrClosedPipe)
	}()

	buf := bytes.NewBuffer(nil)
	_, err := io.Copy(buf, s)
	if err != io.ErrClosedPipe {
		t.Errorf("expect err %v but got %v", io.ErrClosedPipe, err)
	}
	expected := "hello-fragmented-message"
	if buf.String() != expected {
		t.Errorf("expect %s but got %v", expected, buf.String())
	}
}

func TestLocalInput(t *testing.T) {
	r := bytes.NewBufferString("whoami")
	in := NewLocalInput(r)
	out := bytes.NewBuffer(nil)
	io.Copy(out, in)

	if out.String() != "d|whoami" {
		t.Errorf("expect %s but got %s", "d|whoami", out.String())
	}
}

func TestLocalOutput(t *testing.T) {
	testCases := []struct {
		messages []string
		expected string
	}{
		{[]string{"d|root"}, "root"},
		// ignore control messages
		{[]string{"c|close"}, ""},
		{[]string{"r|30 20"}, ""},
		{[]string{"d|root", "c|close"}, "root"},
		// handle fragmened messages
		{[]string{"d|root", "tmp"}, "roottmp"},
	}

	for _, tc := range testCases {
		w := bytes.NewBuffer(nil)
		out := NewLocalOutput(w)

		for _, msg := range tc.messages {
			_, err := out.Write([]byte(msg))
			if err != nil {
				t.Errorf("unexpected error %s", err)
			}
		}

		if w.String() != tc.expected {
			t.Errorf("expect %s but got %s", tc.expected, w.String())
		}
	}
}

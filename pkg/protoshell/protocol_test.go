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

func TestNewShellWriter(t *testing.T) {
	testCases := []struct {
		msg      string
		expected string
	}{
		{`action@foobar`, `d|action@foobar`},
	}

	for _, tc := range testCases {
		w := bytes.NewBuffer(nil)
		s := NewShellWriter(w)

		r := bytes.NewBufferString(tc.msg)
		io.Copy(s, r)

		if w.String() != tc.expected {
			t.Errorf("expect %s to be written but got %s", tc.expected, w.String())
		}
	}
}

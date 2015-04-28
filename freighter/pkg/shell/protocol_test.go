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
	io.Closer
}

func (ws wsReader) NextReader() (int, io.Reader, error) {
	return websocket.BinaryMessage, ws.Reader, nil
}

type nBytesWriter struct {
	N int
}

func (w *nBytesWriter) Write(p []byte) (n int, err error) {
	if w.N < len(p) {
		return w.N, io.ErrShortWrite
	}
	return len(p), nil
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
		{`c|`, ``, &CloseMsg{}},
	}

	for _, tc := range testCases {
		in := bytes.NewBufferString(tc.msg)
		ch := make(chan Message, 1)
		s := NewShellReader(wsReader{Reader: in}, ch, websocket.BinaryMessage)

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
	r, w := io.Pipe()
	s := NewShellReader(wsReader{Reader: r}, make(chan Message), websocket.BinaryMessage)

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
		{`nitrous@foobar`, `d|nitrous@foobar`},
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

func TestShellWriterWrite(t *testing.T) {
	testCases := []struct {
		w    *nBytesWriter
		data string
		n    int
		err  error
	}{
		{&nBytesWriter{0}, "foo", 0, io.ErrShortWrite},
		{&nBytesWriter{1}, "foo", 0, io.ErrShortWrite}, // written: `d`
		{&nBytesWriter{2}, "foo", 0, io.ErrShortWrite}, // written: `d|`
		{&nBytesWriter{3}, "foo", 1, io.ErrShortWrite}, // written: `d|f`
		{&nBytesWriter{5}, "foo", 3, nil},              // written: `d|foo`
	}

	for _, tc := range testCases {
		s := NewShellWriter(tc.w)
		n, err := s.Write([]byte(tc.data))

		if n != tc.n {
			t.Errorf("expected %d bytes to have been written, got %d", tc.n, n)
		}
		if err != tc.err {
			t.Errorf("expected error %+v, got %+v", tc.err, err)
		}
	}
}

func TestShellWriterClose(t *testing.T) {
	w := bytes.NewBuffer(nil)
	s := NewShellWriter(w)

	s.Close()

	if w.String() != `c|` {
		t.Errorf("expect close message to be written but got %s", w.String())
	}
}

package shell

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"time"
)

func TestNewShellReader(t *testing.T) {
	testCases := []struct {
		msg         string
		expected    string
		expectedMsg Message
	}{
		{`p|whoami`, `whoami`, nil},
		{`p|ls`, `ls`, nil},
		{`w|80 20`, ``, &ResizeMsg{width: 80, height: 20}},
		{`c|gone away for good`, ``, &CloseMsg{msg: `gone away for good`}},
	}

	for _, tc := range testCases {
		ch := make(chan Message, 1)
		in := bytes.NewBufferString(tc.msg)
		s := NewShellReader(in, ch)

		out := bytes.NewBuffer(nil)
		n, _ := io.Copy(out, s)

		if n != int64(len(tc.expected)) {
			t.Errorf("expect to read %d bytes but only %d bytes", len(tc.msg), n)
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

		if out.String() != tc.expected {
			t.Errorf("expect ouput %s but got %s", tc.expected, out.String())
		}
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

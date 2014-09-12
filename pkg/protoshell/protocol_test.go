package protoshell

import (
	"bytes"
	"io"
	"testing"
)

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

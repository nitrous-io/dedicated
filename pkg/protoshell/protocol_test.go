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

	if out.String() != "p|whoami" {
		t.Errorf("expect %s but got %s", "p|whoami", out.String())
	}
}

func TestLocalOutput(t *testing.T) {
	w := bytes.NewBuffer(nil)
	out := NewLocalOutput(w)
	io.Copy(out, bytes.NewBufferString("d|root"))

	if w.String() != "root" {
		t.Errorf("expect %s but got %s", "root", w.String())
	}
}

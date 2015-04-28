package copybuffer

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

var errBufClosed = errors.New("buffer closed")

type TestBuffer struct {
	closed bool
	*bytes.Buffer
}

func (b *TestBuffer) Close() error {
	b.closed = true
	return nil
}

func (b *TestBuffer) Write(p []byte) (n int, err error) {
	if b.closed {
		return 0, errBufClosed
	}
	return b.Buffer.Write(p)
}

func TestCopyBuffer(t *testing.T) {
	dst := &TestBuffer{Buffer: &bytes.Buffer{}}
	srcR, srcW := io.Pipe()

	cb := &CopyBuffer{Src: srcR}

	var (
		written int64
		err     error
	)
	go func() {
		written, err = cb.WriteTo(dst)
	}()

	// dst is available
	srcW.Write([]byte("foo"))

	// foo should have been copied
	if actual, expected := dst.String(), "foo"; actual != expected {
		t.Fatalf("expected %s, got %s", expected, actual)
	}

	// buffer should be empty
	if actual, expected := cb.buf.Len(), 0; actual != expected {
		t.Fatalf("expected %d, got %d", expected, actual)
	}

	// dst is closed
	dst.Reset()
	dst.Close()

	// dst is replaced
	newDst := &TestBuffer{Buffer: &bytes.Buffer{}}

	srcW.Write([]byte("bar"))

	// no additional bytes should have been written to dst
	if actual, expected := dst.String(), ""; actual != expected {
		t.Fatalf("expected %s, got %s", expected, actual)
	}

	// no bytes should have been written to new dst
	if actual, expected := newDst.String(), ""; actual != expected {
		t.Fatalf("expected %s, got %s", expected, actual)
	}

	// WriteTo should exit
	if written != 3 {
		t.Fatalf("expected 3 bytes to have been written so far")
	}

	if err != errBufClosed {
		t.Fatalf("expected buffer closed error")
	}

	// buffer should contain the unwritten bytes
	if actual, expected := cb.buf.String(), "bar"; actual != expected {
		t.Fatalf("expected %s, got %s", expected, actual)
	}

	go func() {
		written, err = cb.WriteTo(newDst)
	}()

	srcW.Write([]byte("baz"))

	// previously buffered bar and newly read baz should be copied
	if actual, expected := newDst.String(), "barbaz"; actual != expected {
		t.Fatalf("expected %s, got %s", expected, actual)
	}

	// buffer should be empty
	if actual, expected := cb.buf.Len(), 0; actual != expected {
		t.Fatalf("expected %d, got %d", expected, actual)
	}

	// make WriteTo finish again
	newDst.Close()
	srcW.Write([]byte("X"))

	if written != 6 {
		t.Fatalf("expected 6 bytes to have been written so far")
	}

	if err != errBufClosed {
		t.Fatalf("expected buffer closed error")
	}
}

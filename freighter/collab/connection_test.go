package collab_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/nitrous-io/dedicated/freighter/collab"
)

type Writer struct {
	Written [][]byte
}

func (w *Writer) Write(p []byte) (n int, err error) {
	w.Written = append(w.Written, p)
	return len(p), nil
}

type ErroringWriter struct{}

func (w *ErroringWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("error")
}

func TestNewConnection(t *testing.T) {
	b := &Writer{}

	// when id is specified
	c := collab.NewConnection(b, "foo", "myid")

	if actual, expected := reflect.TypeOf(c), reflect.TypeOf(&collab.Connection{}); expected != actual {
		t.Fatalf("expected type %v, got %v", expected, actual)
	}

	if actual, expected := c.Conn, b; !reflect.DeepEqual(expected, actual) {
		t.Errorf("expected Conn to equal %s, got %s", expected, actual)
	}

	if actual, expected := c.Username, "foo"; expected != actual {
		t.Errorf("expected Username to equal %s, got %s", expected, actual)
	}

	if actual, expected := c.ID, "myid"; expected != actual {
		t.Errorf("expected ID to equal %s, got %s", expected, actual)
	}

	// when id is not specified
	c = collab.NewConnection(b, "foo", "")
	c2 := collab.NewConnection(b, "foo", "")
	c3 := collab.NewConnection(b, "foo", "")

	if c.ID == c2.ID || c.ID == c3.ID || c2.ID == c3.ID {
		t.Errorf("expected unique ID to be generated for each connection")
	}
}

func TestSend(t *testing.T) {
	b := &Writer{}
	c := collab.NewConnection(b, "user", "")

	data := map[string]interface{}{
		"bar": "baz",
		"qux": float64(123),
	}
	err := c.Send("foo.rb", "foo", data)
	if err != nil {
		t.Fatalf("this should not happen: %v", err)
	}

	if actual, expected := string(b.Written[0]), `d|["foo.rb","foo",{"bar":"baz","qux":123}]`+"\n"; actual != expected {
		t.Errorf("expected %s to have been written, got %s", expected, actual)
	}

	// when conn.Write fails, it should buffer and attempt to send
	// the buffered writes on next Send.
	b.Written = [][]byte{}
	c.Conn = &ErroringWriter{}

	err = c.Send("foo.rb", "e", "hello??")
	if err == nil {
		t.Fatalf("this should not happen")
	}
	err = c.Send("foo.rb", "e", "what's going on??")
	if err == nil {
		t.Fatalf("this should not happen")
	}

	if len(b.Written) > 0 {
		t.Errorf("nothing should have been written to buffer yet")
	}

	c.Conn = b
	err = c.Send("foo.rb", "e", "ok phew")
	if err != nil {
		t.Fatalf("this should not happen: %v", err)
	}

	if len(b.Written) != 3 {
		t.Fatalf("3 writes should have happened")
	}

	if actual, expected := string(b.Written[0]), `d|["foo.rb","e","hello??"]`+"\n"; actual != expected {
		t.Errorf("expected %s to have been written, got %s", expected, actual)
	}
	if actual, expected := string(b.Written[1]), `d|["foo.rb","e","what's going on??"]`+"\n"; actual != expected {
		t.Errorf("expected %s to have been written, got %s", expected, actual)
	}
	if actual, expected := string(b.Written[2]), `d|["foo.rb","e","ok phew"]`+"\n"; actual != expected {
		t.Errorf("expected %s to have been written, got %s", expected, actual)
	}
}

func TestBroadcast(t *testing.T) {
	collab.Sessions = map[string]*collab.Session{}

	b := &Writer{}
	b2 := &Writer{}
	b3 := &Writer{}

	s := collab.NewSession("foo.rb", "")
	c := collab.NewConnection(b, "u1", "")
	c2 := collab.NewConnection(b2, "u2", "")
	c3 := collab.NewConnection(b3, "u3", "")

	s.RegisterConnection(c)
	s.RegisterConnection(c2)
	s.RegisterConnection(c3)

	data := map[string]interface{}{
		"bar": "baz",
		"qux": float64(123),
	}
	c.Broadcast(s, "foo", data)

	if len(b.Written) != 0 {
		t.Errorf("nothing should have been written to connection 1's buffer")
	}

	expected := []interface{}{
		s.ID,
		"foo",
		data,
	}

	for _, b := range []*Writer{b2, b3} {
		var d []interface{}
		j := b.Written[0]
		if actual := string(j[:2]); actual != "d|" {
			t.Fatalf(`should begin with "d|", got %s`, actual)
		}
		if err := json.Unmarshal(j[2:], &d); err != nil {
			t.Fatalf("expected no error, got +%v", err)
		}
		if !reflect.DeepEqual(d, expected) {
			t.Errorf("expected %+v to equal %+v", d, expected)
		}
	}
}

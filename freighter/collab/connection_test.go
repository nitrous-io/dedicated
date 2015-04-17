package collab_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nitrous-io/dedicated/freighter/collab"
)

func TestNewConnection(t *testing.T) {
	// TODO
}

func TestSend(t *testing.T) {
	b := &bytes.Buffer{}
	s := collab.NewSession("foo.rb", "")
	c := collab.NewConnection("", b)
	s.RegisterConnection(c)

	data := map[string]interface{}{
		"bar": "baz",
		"qux": float64(123),
	}
	err := c.Send(s, "foo", data)
	if err != nil {
		t.Fatalf("expected no error, got +%v", err)
	}

	var d []interface{}
	j := b.Bytes()
	if actual := string(j[:2]); actual != "d|" {
		t.Fatalf(`should begin with "d|", got +s`, actual)
	}
	if err := json.Unmarshal(j[2:], &d); err != nil {
		t.Fatalf("expected no error, got +%v", err)
	}

	expected := []interface{}{
		s.ID,
		"foo",
		data,
	}
	if !reflect.DeepEqual(d, expected) {
		t.Errorf("expected %+v to equal %+v", d, expected)
	}
}

func TestBroadcast(t *testing.T) {
	collab.Sessions = map[string]*collab.Session{}

	b := &bytes.Buffer{}
	b2 := &bytes.Buffer{}
	b3 := &bytes.Buffer{}

	s := collab.NewSession("foo.rb", "")
	c := &collab.Connection{Conn: b}
	c2 := &collab.Connection{Conn: b2}
	c3 := &collab.Connection{Conn: b3}

	s.RegisterConnection(c)
	s.RegisterConnection(c2)
	s.RegisterConnection(c3)

	data := map[string]interface{}{
		"bar": "baz",
		"qux": float64(123),
	}
	c.Broadcast(s, "foo", data)

	if b.Len() != 0 {
		t.Errorf("nothing should have been written to connection 1's buffer")
	}

	expected := []interface{}{
		s.ID,
		"foo",
		data,
	}

	for _, b := range []*bytes.Buffer{b2, b3} {
		var d []interface{}
		j := b.Bytes()
		if actual := string(j[:2]); actual != "d|" {
			t.Fatalf(`should begin with "d|", got +s`, actual)
		}
		if err := json.Unmarshal(j[2:], &d); err != nil {
			t.Fatalf("expected no error, got +%v", err)
		}
		if !reflect.DeepEqual(d, expected) {
			t.Errorf("expected %+v to equal %+v", d, expected)
		}
	}
}

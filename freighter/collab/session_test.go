package collab_test

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/nitrous-io/dedicated/freighter/collab"
)

func TestNewSession(t *testing.T) {
	collab.Sessions = map[string]*collab.Session{}

	s := collab.NewSession("foo.rb", `puts "hello world"\n`)
	if s == nil {
		t.Errorf("expected not nil, got nil")
	}
	if actual, expected := reflect.TypeOf(s), reflect.TypeOf(&collab.Session{}); expected != actual {
		t.Fatalf("expected type %v, got %v", expected, actual)
	}

	if s.Session == nil {
		t.Errorf("embedded ot/session should have been initialized")
	}

	s2 := collab.NewSession("bar.c", `#include <stdio.h>`)
	if s == nil {
		t.Errorf("expected not nil, got nil")
	}

	if s == s2 {
		t.Errorf("two sessions should be different")
	}

	s3 := collab.NewSession("foo.rb", `something...`)
	if s != s3 {
		t.Errorf("expected same session, got different session")
	}
}

func TestGetSession(t *testing.T) {
	collab.Sessions = map[string]*collab.Session{}

	s := collab.NewSession("foo.rb", `puts "hello world"\n`)
	if s == nil {
		t.Errorf("expected not nil, got nil")
	}

	s2 := collab.NewSession("bar.c", `#include <stdio.h>`)
	if s == nil {
		t.Errorf("expected not nil, got nil")
	}

	ss := collab.GetSession("foo.rb")
	if expected, actual := s, ss; expected != actual {
		t.Errorf("expected %+v, got %+v", expected, actual)
	}

	ss = collab.GetSession("bar.c")
	if expected, actual := s2, ss; expected != actual {
		t.Errorf("expected %+v, got %+v", expected, actual)
	}

	ss = collab.GetSession("baz.c")
	if actual := ss; actual != nil {
		t.Errorf("expected nil, got %+v", actual)
	}
}

func TestRegisterConnection(t *testing.T) {
	collab.Sessions = map[string]*collab.Session{}
	s := collab.NewSession("foo.rb", "")
	if s == nil {
		t.Fatalf("should not be nil")
	}

	c := &collab.Connection{}
	s.RegisterConnection(c)

	if len(s.Connections) != 0 {
		t.Errorf("connection with no ID should not be registerable")
	}

	c = &collab.Connection{ID: "1"}

	if s.Connections[c.ID] == c {
		t.Errorf("connection should not have been registered yet")
	}

	s.RegisterConnection(c)

	if s.Connections[c.ID] != c {
		t.Errorf("connection should have been registered")
	}

	c2 := &collab.Connection{ID: "2"}
	s.RegisterConnection(c2)

	if s.Connections[c.ID] != c {
		t.Errorf("previous connection should still be registered")
	}

	if s.Connections[c2.ID] != c2 {
		t.Errorf("new connection should also be registered")
	}
}

func TestUnregisterConnection(t *testing.T) {
	id := "foo.rb"
	collab.Sessions = map[string]*collab.Session{}
	s := collab.NewSession(id, "")
	if s == nil {
		t.Fatalf("should not be nil")
	}

	c := &collab.Connection{ID: "1"}
	c2 := &collab.Connection{ID: "2"}
	c3 := &collab.Connection{ID: "3"}
	s.RegisterConnection(c)
	s.RegisterConnection(c2)
	s.RegisterConnection(c3)

	if s.Connections[c3.ID] != c3 {
		t.Errorf("connection 3 should be registered")
	}
	if collab.GetSession(id) == nil {
		t.Errorf("session should still exist")
	}

	s.UnregisterConnection(c3)

	if s.Connections[c.ID] != c {
		t.Errorf("connection 1 should still be registered")
	}
	if s.Connections[c2.ID] != c2 {
		t.Errorf("connection 2 should still be registered")
	}
	if _, ok := s.Connections[c3.ID]; ok {
		t.Errorf("connection 3 should no longer be registered")
	}
	if collab.GetSession(id) == nil {
		t.Errorf("session should still exist")
	}

	s.UnregisterConnection(c2)

	if _, ok := s.Connections[c2.ID]; ok {
		t.Errorf("connection 2 should no longer be registered")
	}
	if collab.GetSession(id) == nil {
		t.Errorf("session should still exist")
	}

	s.UnregisterConnection(c)
	runtime.Gosched()

	if collab.GetSession(id) != nil {
		t.Errorf("session should no longer exist")
	}
}

func TestUnregisterConnectionGlobally(t *testing.T) {
	collab.Sessions = map[string]*collab.Session{}

	s := collab.NewSession("foo.rb", "")
	s2 := collab.NewSession("bar.rb", "")

	c := &collab.Connection{ID: "1"}
	c2 := &collab.Connection{ID: "2"}
	c3 := &collab.Connection{ID: "3"}

	s.RegisterConnection(c)
	s.RegisterConnection(c2)
	s.RegisterConnection(c3)
	s2.RegisterConnection(c)
	s2.RegisterConnection(c2)
	s2.RegisterConnection(c3)

	collab.UnregisterConnectionGlobally(c2)
	if _, ok := s.Connections[c2.ID]; ok {
		t.Errorf("connection 2 should no longer be registered in session 1")
	}
	if _, ok := s2.Connections[c2.ID]; ok {
		t.Errorf("connection 2 should no longer be registered in session 2")
	}

	if s.Connections[c.ID] != c {
		t.Errorf("connection 1 is not affected in session 1")
	}
	if s.Connections[c3.ID] != c3 {
		t.Errorf("connection 3 is not affected in session 1")
	}

	if s2.Connections[c.ID] != c {
		t.Errorf("connection 1 is not affected in session 2")
	}
	if s2.Connections[c3.ID] != c3 {
		t.Errorf("connection 3 is not affected in session 2")
	}

	// the following two tests a potential deadlock that happens
	// if UnregisterConnection called by UnregisterConnectionGlobally
	// also locks sessionsLock
	collab.UnregisterConnectionGlobally(c3)
	collab.UnregisterConnectionGlobally(c)
}

func TestUpdateConnection(t *testing.T) {
	collab.Sessions = map[string]*collab.Session{}

	s := collab.NewSession("foo.rb", "")
	s2 := collab.NewSession("bar.rb", "")

	c := &collab.Connection{ID: "1"}
	c2 := &collab.Connection{ID: "2"}
	c3 := &collab.Connection{ID: "3"}

	s.RegisterConnection(c)
	s.RegisterConnection(c2)
	s.RegisterConnection(c3)
	s2.RegisterConnection(c)
	s2.RegisterConnection(c2)
	s2.RegisterConnection(c3)

	c4 := &collab.Connection{ID: c2.ID}
	collab.UpdateConnection(c4)

	if s.Connections[c2.ID] != c4 {
		t.Errorf("connection 2 in session 1 should be replaced with connection 4")
	}
	if s2.Connections[c2.ID] != c4 {
		t.Errorf("connection 2 in session 2 should be replaced with connection 4")
	}

	if s.Connections[c.ID] != c {
		t.Errorf("connection 1 is not affected in session 1")
	}
	if s.Connections[c3.ID] != c3 {
		t.Errorf("connection 3 is not affected in session 1")
	}

	if s2.Connections[c.ID] != c {
		t.Errorf("connection 1 is not affected in session 2")
	}
	if s2.Connections[c3.ID] != c3 {
		t.Errorf("connection 3 is not affected in session 2")
	}
}

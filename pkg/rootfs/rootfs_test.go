package rootfs

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func setup() *Rootfs {
	d, _ := ioutil.TempDir("/tmp", "test-root")
	return NewRootfs(d)
}

func assertExists(t *testing.T, p string) {
	stat, err := os.Stat(p)
	if err != nil {
		t.Fatalf("%s should exist, %v %s", p, stat, err)
	}
}

func assertNotExists(t *testing.T, p string) {
	stat, err := os.Stat(p)
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("%s should exist, %v %s", p, stat, err)
	}
}

func TestCreate(t *testing.T) {
	r := setup()
	defer os.RemoveAll(r.root)

	cases := []struct {
		name string
		err  error
	}{
		{"test", nil},
		{"../test", ErrInvalidPath},
	}

	for _, c := range cases {
		_, err := r.Create(c.name)
		if err != nil {
			if c.err != nil {
				if err != c.err {
					t.Errorf("path %q expects err(%s), got %s", c.name, c.err, err)
				}
			} else {
				t.Errorf("path %q got err %s", c.name, err)
			}
		}
		os.RemoveAll(r.root)
	}
}

func TestCreateNew(t *testing.T) {
	r := setup()
	defer os.RemoveAll(r.root)

	f, err := r.CreateNew("test")
	if err != nil || f == nil {
		t.Fatalf("Unexpected %v %v", f, err)
	}
	f, err = r.CreateNew("test")
	if err == nil || f != nil {
		t.Fatalf("Unexpected %v %v", f, err)
	}
}

func TestOpen(t *testing.T) {
	r := setup()
	defer os.RemoveAll(r.root)

	f, _ := r.Create("test")
	fmt.Fprint(f, "Hello world")
	f, err := r.Open("test")
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	b := bytes.NewBuffer(nil)
	io.Copy(b, f)
	if c := b.String(); c != "Hello world" {
		t.Fatalf("Unexpected content %v", c)
	}
}

func TestMkdir(t *testing.T) {
	r := setup()
	defer os.RemoveAll(r.root)
	err := r.Mkdir("test")
	if err != nil {
		t.Fatalf("Unexpected error", err)
	}
	assertExists(t, path.Join(r.root, "test"))
}

func TestRename(t *testing.T) {
	r := setup()
	defer os.RemoveAll(r.root)
	r.Mkdir("test")
	if err := r.Rename("test", "hello"); err != nil {
		t.Fatalf("Unexpected error", err)
	}
	assertExists(t, path.Join(r.root, "hello"))
	assertNotExists(t, path.Join(r.root, "test"))
}

func TestRemove(t *testing.T) {
	r := setup()
	defer os.RemoveAll(r.root)
	r.Mkdir("test")
	if err := r.Remove("test"); err != nil {
		t.Fatalf("Unexpected error", err)
	}
	assertNotExists(t, path.Join(r.root, "test"))
}

func TestReadDir(t *testing.T) {
	r := setup()
	defer os.RemoveAll(r.root)

	r.Mkdir("test")
	r.Mkdir("test/hello")
	r.Mkdir("test/world")
	entries, err := r.ReadDir("test")
	if err != nil || len(entries) != 2 {
		t.Fatalf("Unexpected error", entries, err)
	}
}

func TestStat(t *testing.T) {
	r := setup()
	defer os.RemoveAll(r.root)
	r.Mkdir("test")
	stat, err := r.Stat("test")
	if err != nil || stat.Name() != "test" {
		t.Fatalf("Unexpected error", stat, err)
	}
}

func TestPath(t *testing.T) {
	cases := []struct {
		name     string
		expected string
		err      error
	}{
		{".", "/root", nil},
		{"./hello", "/root/hello", nil},
		{"./hello/test", "/root/hello/test", nil},
		{"../hello/test", "", ErrInvalidPath},
		{"./hello/../../test", "", ErrInvalidPath},
	}

	r := NewRootfs("/root")

	for _, c := range cases {
		p, err := r.path(c.name)
		if p != c.expected || err != c.err {
			t.Fatalf("Given (%s), got Path(%s) with err(%s), expected path(%s) with err(%s)",
				c.name, p, err, c.expected, c.err)
		}
	}
}

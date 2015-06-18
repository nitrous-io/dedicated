package cops

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sirupsen/logrus"
)

func init() { logrus.SetLevel(logrus.PanicLevel) }

func TestGetCopsInfo(t *testing.T) {
	// save current value to restore later
	newState, oldState := newStateDir, oldStateDir
	defer func() {
		newStateDir, oldStateDir = newState, oldState
	}()

	// create tmp directory for storing states
	tmpNewState, err := ioutil.TempDir("", "new-state")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpNewState)
	tmpOldState, err := ioutil.TempDir("", "old-state")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpOldState)

	// assign global var to new tmp vars
	newStateDir, oldStateDir = tmpNewState, tmpOldState

	var stateFile string
	os.MkdirAll(filepath.Join(newStateDir, "123"), 0755)
	stateFile = filepath.Join(newStateDir, "123", "state.json")
	if err := ioutil.WriteFile(stateFile, []byte(fmt.Sprintf(`{"init_process_pid": %d }`, os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}

	info, err := getCopsInfo("123", nil)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if euid := os.Getuid(); info.UID != euid {
		t.Errorf("uid(%d), expect %d", info.UID, euid)
	}
	if egid := os.Getgid(); info.GID != egid {
		t.Errorf("gid(%d), expect %d", info.GID, egid)
	}
}

func TestParsePasswdFile(t *testing.T) {
	// a subset of real /etc/passwd
	s := `root:x:0:0:root:/root:/bin/bash
sync:x:4:65534:sync:/bin:/bin/sync
games:x:5:60:games:/usr/games:/usr/sbin/nologin
man:x:6:12:man:/var/cache/man:/usr/sbin/nologin
dnsmasq:x:103:65534:dnsmasq,,,:/var/lib/misc:/bin/false
lxc-dnsmasq:x:104:106:LXC dnsmasq,,,:/var/lib/lxc:/bin/false
postgres:x:105:108:PostgreSQL administrator,,,:/var/lib/postgresql:/bin/bash

invalid-uid:x:invaliduid:100:LXC dnsmasq,,,:/var/lib/lxc:/bin/false
invalid-gid:x:100:invalidgid:LXC dnsmasq,,,:/var/lib/lxc:/bin/false

dup-user:x:1000:1000::/home/nitrous:/bin/zsh
dup-user:x:1001:1001::/home/nitrous:/bin/zsh
`

	f, err := ioutil.TempFile("", "etcpasswd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	fmt.Fprint(f, s)

	var uid, gid int

	// for a normal user
	uid, gid, err = parsePasswdFile(f.Name(), "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if uid != 105 || gid != 108 {
		t.Errorf("unexpected uid(%d), gid(%d)", uid, gid)
	}

	// for duplicate entries in /etc/passwd
	uid, gid, err = parsePasswdFile(f.Name(), "dup-user")
	if err != nil {
		t.Fatal(err)
	}
	if uid != 1001 || gid != 1001 {
		t.Errorf("unexpected uid(%d), gid(%d)", uid, gid)
	}

	// for invalid usernames
	for _, username := range []string{
		"missing-user",
		"invalid-uid",
		"invalid-gid",
	} {
		uid, gid, err = parsePasswdFile(f.Name(), "missing-user")
		if err == nil || uid != -1 || gid != -1 {
			t.Fatalf("Expect error for user %s", username)
		}
	}
}

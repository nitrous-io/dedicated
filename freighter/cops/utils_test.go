package cops

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetContainerPID(t *testing.T) {
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

	// test new state path
	var stateFile string
	os.MkdirAll(filepath.Join(newStateDir, "123"), 0755)
	stateFile = filepath.Join(newStateDir, "123", "state.json")
	if err := ioutil.WriteFile(stateFile, []byte(`{"init_process_pid": 234}`), 0644); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(oldStateDir, "987"), 0755)
	stateFile = filepath.Join(oldStateDir, "987", "state.json")
	if err := ioutil.WriteFile(stateFile, []byte(`{"init_pid": 765}`), 0644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		containerID string
		pid         int
		hasError    bool
	}{
		{"123", 234, false},
		{"987", 765, false},
		{"300", 0, true},
	} {
		pid, err := getContainerPID(tc.containerID)
		if err != nil && !tc.hasError {
			t.Errorf("failed to get pid %v", err)
		}
		if pid != tc.pid {
			t.Errorf("pid(%d), expected %d", pid, tc.pid)
		}
	}
}

func TestRemoveBogusFromStderr(t *testing.T) {
	tests := []string{
		"nvm_supports_source_options:.:1: no such file or directory: /dev/stdin\n",
		"",

		"nvm_supports_source_options:.:1: no such file or directory: /dev/stdin\nthe real thing",
		"the real thing",

		"nvm_supports_source_options:.:1: no such file or directory: /dev/stdin",
		"nvm_supports_source_options:.:1: no such file or directory: /dev/stdin",

		"/usr/local/opt/nvm/nvm.sh: line 1550: /dev/stdin: No such file or directory\n",
		"",

		"/usr/local/opt/nvm/nvm.sh: line 1550: /dev/stdin: No such file or directory\n the real thing\n",
		" the real thing\n",
	}

	n := len(tests) / 2
	for i := 0; i < n; i++ {
		test := tests[2*i]
		exp := tests[2*i+1]
		got := removeBogusFromStderr(test)
		if got != exp {
			t.Errorf("for removeBogusFromStderr('%s'), got '%s', expected '%s'", test, got, exp)
		}
	}
}

func ensureGotShell(t *testing.T, user, got, exp string) {
	if got != exp {
		t.Errorf("for user '%s' got '%s', expected '%s'", user, got, exp)
	}
}

func TestExtractShellFromEtcPasswd(t *testing.T) {
	// a subset of real /etc/passwd
	s := `root:x:0:0:root:/root:/bin/bash
sync:x:4:65534:sync:/bin:/bin/sync
games:x:5:60:games:/usr/games:/usr/sbin/nologin
man:x:6:12:man:/var/cache/man:/usr/sbin/nologin
nitrous:x:1000:1000::/home/nitrous:/bin/zsh
dnsmasq:x:103:65534:dnsmasq,,,:/var/lib/misc:/bin/false
lxc-dnsmasq:x:104:106:LXC dnsmasq,,,:/var/lib/lxc:/bin/false
postgres:x:105:108:PostgreSQL administrator,,,:/var/lib/postgresql:/bin/bash`
	shell := extractShellFromEtcPasswd(s, "root")
	ensureGotShell(t, "root", shell, "/bin/bash")
	shell = extractShellFromEtcPasswd(s, "nitrous")
	ensureGotShell(t, "nitrous", shell, "/bin/zsh")
	// unkown shell gets converted to /bin/bash
	shell = extractShellFromEtcPasswd(s, "sync")
	ensureGotShell(t, "nitrous", shell, "/bin/bash")
}

func verifyGetArgAsString(t *testing.T, test interface{}, exp []string, expOk bool) {
	got, ok := getArgAsStrArray(test)
	if ok != expOk {
		t.Errorf("for '%#v' got ok %v, expected ok %v, got value '%#v'", test, ok, expOk, got)
		return
	}
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("for '%#v' got '%v', expected '%v'", test, got, exp)
	}
}

func TestGetArgAsStrArray(t *testing.T) {
	verifyGetArgAsString(t, 5, nil, false)
	verifyGetArgAsString(t, []string{}, nil, false)
	verifyGetArgAsString(t, []int{}, nil, false)
	verifyGetArgAsString(t, []int{5}, nil, false)
	verifyGetArgAsString(t, []interface{}{"5", "3"}, []string{"5", "3"}, true)
}

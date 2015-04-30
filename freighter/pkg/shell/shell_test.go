// shell test requires talking to an actual docker daemon
package shell

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	baseOnce sync.Once
	baseErr  error
)

func buildBase() error {
	baseOnce.Do(func() {
		// build base, which is busybox with /sh as CMD
		cmd := exec.Command("docker", "build", "-t", "base:latest", ".")
		cmd.Dir = "/docker-busybox"
		_, baseErr = cmd.CombinedOutput()
	})
	return baseErr
}

// This test simulates the following behavior
// - closing the browser
// - closing the browser tab
// - closing the shell tab
// The current javascript implementation will send close message which then
// trigger sh.Close()
func TestShellAttachThenClose(t *testing.T) {
	if err := buildBase(); err != nil {
		t.Fatalf("failed to build base %v", err)
	}

	out, err := exec.Command("docker", "run", "-d", "--name", "testing", "base:latest", "top").CombinedOutput()
	if err != nil {
		t.Fatalf("failed to start container %s %v", string(out), err)
	}
	defer exec.Command("docker", "rm", "-f", "testing").Run()

	id := strings.TrimSpace(string(out))
	command := "echo -n hello && sleep 10"
	sh, err := OpenShell(id, "", command, false)
	if err != nil {
		t.Fatalf("failed to open a shell %v", err)
	}

	stdinR, _ := io.Pipe()
	stdout := bytes.NewBuffer(nil)
	ch := make(chan error, 1)
	attached := make(chan struct{})
	go func() { ch <- sh.Attach(stdinR, stdout, attached) }()

	select {
	case attached <- <-attached:
		time.Sleep(100 * time.Millisecond)
		sh.Close()
		select {
		case <-ch:
			if output := stdout.String(); output != "hello" {
				t.Fatalf("Output(%q), expect %q", output, "hello")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("timed out waiting for exit")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for attach")
	}
}

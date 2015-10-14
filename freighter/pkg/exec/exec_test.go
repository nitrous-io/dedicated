package exec

import (
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

// 1. Setup an exec instance in the running container
// 2. Run a command
// 3. Close the exec instance
func TestExec(t *testing.T) {
	if err := buildBase(); err != nil {
		t.Fatalf("failed to build base %v", err)
	}

	out, err := exec.Command("docker", "run", "-d", "-t", "-i", "--name", "testing", "base:latest", "top").CombinedOutput()
	if err != nil {
		t.Fatalf("failed to start container %s %v", string(out), err)
	}
	defer exec.Command("docker", "rm", "-f", "testing").Run()

	// Get the container ID
	id := strings.TrimSpace(string(out))

	// Run commands
	command := "echo 'hi' >> /root/test_file && echo 'hello' >> /root/test_file"
	err = RunCommand(id, command, ExecOptions{})
	if err != nil {
		t.Fatalf("failed to run the command: %v", err)
	}

	out, err = exec.Command("docker", "cp", "testing:/root/test_file", "-").Output()
	if err != nil {
		t.Fatalf("failed to read file from container: %s %v", string(out), err)
	}

	// Get the container ID
	expectedOutput := "hi\nhello"
	output := strings.TrimSpace(string(out))

	// It returns the file content in tar archive format
	if !strings.Contains(output, expectedOutput) {
		t.Fatalf("expected output %v to contain %v", output, expectedOutput)
	}
}

func TestExecWithUsername(t *testing.T) {
	if err := buildBase(); err != nil {
		t.Fatalf("failed to build base %v", err)
	}

	// phusion/baseimage is lightest ubuntu image so far
	out, err := exec.Command("docker", "run", "-d", "-t", "-i", "--name", "testing", "phusion/baseimage:latest", "top").CombinedOutput()
	if err != nil {
		t.Fatalf("failed to start container %s %v", string(out), err)
	}
	defer exec.Command("docker", "rm", "-f", "testing").Run()

	// Get the container ID
	id := strings.TrimSpace(string(out))

	// Create a user
	out, err = exec.Command("docker", "exec", "-i", "testing", "/bin/sh", "-c", `adduser --disabled-password --gecos "" nitrous`).CombinedOutput()
	if err != nil {
		t.Fatalf("failed to add a user to container: %s %v", string(out), err)
	}

	// Create a file in nitrous account home folder
	command := "echo 'hello' >> /home/nitrous/test_file"
	err = RunCommand(id, command, ExecOptions{Username: "nitrous"})
	if err != nil {
		t.Fatalf("failed to run the command: %v", err)
	}

	// Get owner and group
	out, err = exec.Command("docker", "exec", "-i", "testing", "/bin/sh", "-c", `ls -al /home/nitrous/test_file`).CombinedOutput()
	if err != nil {
		t.Fatalf("failed to get the file info: %s %v", string(out), err)
	}

	// i.e. -rw-r--r-- 1 nitrous nitrous 6 Oct 14 09:41 /home/nitrous/test_file
	token := strings.Split(string(out), " ")

	if token[2] != "nitrous" {
		t.Fatalf("expected file owner to be nitrous but get %s", token[2])
	}

	if token[3] != "nitrous" {
		t.Fatalf("expected file group to be nitrous but get %s", token[3])
	}
}

func TestExecWithTimeout(t *testing.T) {
	if err := buildBase(); err != nil {
		t.Fatalf("failed to build base %v", err)
	}

	out, err := exec.Command("docker", "run", "-d", "-t", "-i", "--name", "testing", "base:latest", "top").CombinedOutput()
	if err != nil {
		t.Fatalf("failed to start container %s %v", string(out), err)
	}
	defer exec.Command("docker", "rm", "-f", "testing").Run()

	// Get the container ID
	id := strings.TrimSpace(string(out))

	done := make(chan struct{})

	go func() {
		// Create a file after 10 second
		command := "sleep 10 && echo 'hello' >> /root/test_file"
		err = RunCommand(id, command, ExecOptions{Timeout: 100 * time.Millisecond})
		if err != ErrCommandTimeout {
			t.Fatalf("expected timeout error but got: %v", err)
		}

		done <- struct{}{}
	}()

	select {
	case <-done:
		out, err = exec.Command("docker", "cp", "testing:/root/test_file", "-").Output()
		if err == nil {
			t.Fatalf("expect not to create /root/test_file")
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatalf("Timeout waiting to run the command")
	}
}

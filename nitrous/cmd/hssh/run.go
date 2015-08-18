package main

import (
	"io"
	"os"
	"strings"

	"github.com/mitchellh/cli"
)

type runCmd struct{}

func newRunCommand() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &runCmd{}, nil
	}
}

func (s *runCmd) Run(args []string) int {
	if len(args) < 2 {
		return failf("invalid arguments")
	}
	slug := args[0]

	c, err := newConn(slug)
	if err != nil {
		return failf("shell: %q", err)
	}

	session, err := c.Client.NewSession()
	if err != nil {
		return failf("shell: failed to create new session %q", err)
	}
	defer session.Close()

	stdin, stdout, stderr := pipes(session)

	go io.Copy(stdin, os.Stdin)
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	cmd := strings.Join(args[1:], " ")
	if err := session.Start(cmd); err != nil {
		return failf("shell: failed to start %q on host %q, error %q", cmd, slug, err)
	}

	if err := session.Wait(); err != nil {
		return failf("shell: %q on host %q failed, error %q", cmd, slug, err)
	}

	return 0
}

func (s *runCmd) Help() string {
	return "hssh run remote-host-slug cmd..."
}

func (s *runCmd) Synopsis() string {
	return "Run a command on Nitrous.IO remote-host"
}

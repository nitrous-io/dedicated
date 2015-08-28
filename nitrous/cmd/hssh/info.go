package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"time"

	"github.com/mitchellh/cli"

	"github.com/nitrous-io/dedicated/nitrous/pkg/hostkeys"
)

type infoCmd struct{}

func newInfoCommand() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &infoCmd{}, nil
	}
}

func (s *infoCmd) Run(args []string) int {
	if len(args) < 1 {
		return failf("invalid argument: needs host slug")
	}
	slug := args[0]

	h, err := hostkeys.FromSlug(dbconn, slug)
	if err != nil {
		return failf("info: %q", err)
	}

	sshUser := "root"
	if h.Provider == "amazon" {
		sshUser = "ubuntu"
	}

	f, err := ioutil.TempFile(os.TempDir(), "hssh")
	if err != nil {
		return failf("info: %q", err)
	}
	defer f.Close()

	pem, err := h.PrivatePEM()
	if err != nil {
		return failf("info: %q", err)
	}

	if _, err := f.Write(pem); err != nil {
		return failf("info: %q", err)
	}

	rmKey := func() {
		if err := os.Remove(f.Name()); err != nil {
			failf("info: %q", err)
		}
	}

	defer rmKey()

	fmt.Fprintf(os.Stderr, "Private key %s will be removed in 2 minutes!\n", f.Name())
	fmt.Printf("ssh -p 65100 -i %s %s@%s\n", f.Name(),
		sshUser, h.IPAddress.String)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		_ = <-c
		rmKey()
		os.Exit(130)
	}()

	time.Sleep(2 * time.Minute)

	return 0
}

func (s *infoCmd) Help() string {
	return "hssh info remote-host-slug"
}

func (s *infoCmd) Synopsis() string {
	return "Saves private key to file and outputs command to ssh to remote-host"
}

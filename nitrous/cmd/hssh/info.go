package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"time"

	"github.com/mitchellh/cli"
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

	h, err := hostFromSlug(slug)
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

	privKey, err := h.privateKey()
	if err != nil {
		return failf("info: %q", err)
	}

	if _, err := f.Write(privatePEM(privKey)); err != nil {
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

func privatePEM(priv *rsa.PrivateKey) []byte {
	// ASN.1 DER encoded form
	privder := x509.MarshalPKCS1PrivateKey(priv)
	privblk := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privder,
	}
	return pem.EncodeToMemory(&privblk)
}

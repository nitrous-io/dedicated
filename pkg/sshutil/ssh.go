package sshutil

import (
	"bufio"
	"errors"
	"io"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

type Conn struct {
	Client  *ssh.Client
	Config  *ssh.ClientConfig
	Address string
}

func (c *Conn) Upload(path string, input io.Reader) error {
	// The target directory and file for talking the SCP protocol
	targetDir := filepath.Dir(path)
	targetFile := filepath.Base(path)
	scpFunc := func(w io.Writer, stdoutR *bufio.Reader) error {
		return scpUploadFile(targetFile, input, w, stdoutR)
	}
	return c.scpSession("scp -vt "+targetDir, scpFunc)
}

func (c *Conn) Execute(cmd string) error {
	session, err := c.newSession()
	if err != nil {
		return err
	}
	defer session.Close()

	return session.Run(cmd)
}

func (c *Conn) CombinedOutput(cmd string) ([]byte, error) {
	session, err := c.newSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	return session.CombinedOutput(cmd)
}

func (c *Conn) Close() {
	if c.Client != nil {
		c.Client.Close()
	}
}

func (c *Conn) newSession() (*ssh.Session, error) {
	if c.Client == nil {
		return nil, errors.New("client not available")
	}
	return c.Client.NewSession()
}

// connect to the ssh server.
// if there are existing client, try to close it and reopen a new one
func (c *Conn) connect() error {
	c.Close()
	c.Client = nil
	client, err := ssh.Dial("tcp", c.Address, c.Config)
	if err != nil {
		return err
	}
	c.Client = client
	return nil
}

func NewSSHConn(address string, config *ssh.ClientConfig) (*Conn, error) {
	c := &Conn{
		Address: address,
		Config:  config,
	}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

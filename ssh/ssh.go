package ssh

import "code.google.com/p/go.crypto/ssh"

type Conn struct {
	Client  *ssh.Client
	Config  *ssh.ClientConfig
	Address string
}

// connect to the ssh server.
// if there are existing client, try to close it and reopen a new one
func (c *Conn) connect() error {
	if c.Client != nil {
		c.Client.Close()
	}
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

package amazon

import (
	"crypto/rand"
	"encoding/hex"
	"io"

	"github.com/mitchellh/goamz/ec2"
)

type keypair struct {
	name     string
	provider *ec2.EC2
}

func (k *keypair) Create() error {
	_, err := k.provider.CreateKeyPair(k.name)
	return err
}

func (k *keypair) Delete() error {
	_, err := k.provider.DeleteKeyPair(k.name)
	return err
}

func generateTempKeypair(conn *ec2.EC2) (*keypair, error) {
	id := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		panic(err) // This shouldn't happen
	}
	k := &keypair{name: hex.EncodeToString(id), provider: conn}
	if err := k.Create(); err != nil {
		return nil, err
	}
	return k, nil
}

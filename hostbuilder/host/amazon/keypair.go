package amazon

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"

	"github.com/mitchellh/goamz/ec2"
)

type keypair struct {
	name       string
	privateKey *rsa.PrivateKey
	provider   *ec2.EC2
}

func (k *keypair) Create() error {
	key, err := k.provider.CreateKeyPair(k.name)
	if err != nil {
		return err
	}
	block, _ := pem.Decode([]byte(key.KeyMaterial))
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse a valid RSA private key, err: %s", err)
	}
	k.privateKey = privKey
	return nil
}

func (k *keypair) Delete() error {
	_, err := k.provider.DeleteKeyPair(k.name)
	return err
}

func generateTempKeypair(conn *ec2.EC2) (*keypair, error) {
	id := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		return nil, fmt.Errorf("generateTempKeypair error %s", err)
	}
	k := &keypair{name: hex.EncodeToString(id), provider: conn}
	if err := k.Create(); err != nil {
		return nil, err
	}
	return k, nil
}

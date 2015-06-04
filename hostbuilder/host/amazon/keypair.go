package amazon

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type keypair struct {
	name       string
	privateKey *rsa.PrivateKey
	provider   *ec2.EC2
}

func (k *keypair) Create() error {
	key, err := k.provider.CreateKeyPair(&ec2.CreateKeyPairInput{
		KeyName: aws.String(k.name),
	})
	if err != nil {
		return err
	}
	if key.KeyMaterial == nil {
		return fmt.Errorf("invalid key response %+v", key)
	}
	block, _ := pem.Decode([]byte(*key.KeyMaterial))
	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse a valid RSA private key, err: %v", err)
	}
	k.privateKey = privKey
	return nil
}

func (k *keypair) Delete() error {
	_, err := k.provider.DeleteKeyPair(&ec2.DeleteKeyPairInput{
		KeyName: aws.String(k.name),
	})
	return err
}

// generateTempKeypair creates a temporary EC2 keypair that can be used to
// provision an instance. The keypair should be removed after the instance is
// created i.e. it's only used during host creation. A custom keypair should be
// setup on the host to be used afterwards.
func generateTempKeypair(conn *ec2.EC2) (*keypair, error) {
	id := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		return nil, fmt.Errorf("generateTempKeypair error %v", err)
	}
	k := &keypair{name: hex.EncodeToString(id), provider: conn}
	if err := k.Create(); err != nil {
		return nil, err
	}
	return k, nil
}

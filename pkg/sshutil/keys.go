// convinient function to work with keypairs
package sshutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

func GenerateSSHKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

func GetPubKey(priv *rsa.PrivateKey) ([]byte, error) {
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		return []byte{}, err
	}
	return ssh.MarshalAuthorizedKey(pub), nil
}

func GetPem(priv *rsa.PrivateKey) []byte {
	// ASN.1 DER encoded form
	priv_der := x509.MarshalPKCS1PrivateKey(priv)
	priv_blk := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   priv_der,
	}
	return pem.EncodeToMemory(&priv_blk)
}

package provisioner

import (
	"crypto/rsa"
)

func GenerateKey() (priv *rsa.PrivateKey, pub []byte, err error) {
	return generateKey()
}

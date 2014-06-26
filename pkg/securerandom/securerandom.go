package securerandom

import (
	"crypto/rand"
	"encoding/base64"
)

func Base64(n uint) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

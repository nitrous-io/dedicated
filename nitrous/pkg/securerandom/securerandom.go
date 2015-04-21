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

const alphaNumericDict = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// AlphaNumeric generates a random alpha-numeric string of length n.
func AlphaNumeric(n uint) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	bl := byte(len(alphaNumericDict))
	for k, v := range b {
		b[k] = alphaNumericDict[v%bl]
	}
	return string(b), nil
}

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

const (
	numericDict      = "0123456789"
	alphanumericDict = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// Numeric generates a random numeric string of length n.
func Numeric(n uint) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	bl := byte(len(numericDict))
	for k, v := range b {
		b[k] = numericDict[v%bl]
	}
	return string(b), nil
}

// Alphanumeric generates a random alpha-numeric string of length n.
func Alphanumeric(n uint) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	bl := byte(len(alphanumericDict))
	for k, v := range b {
		b[k] = alphanumericDict[v%bl]
	}
	return string(b), nil
}

package bcrypt

import (
	bc "code.google.com/p/go.crypto/bcrypt"
)

func Hash(password string) (string, error) {
	pw_bytes, err := bc.GenerateFromPassword([]byte(password), bc.DefaultCost)
	return string(pw_bytes), err
}

func Verify(hashed string, password string) bool {
	err := bc.CompareHashAndPassword([]byte(hashed), []byte(password))
	return err == nil
}

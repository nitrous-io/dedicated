package uuid

import (
	"crypto/rand"
	"encoding/hex"
)

func Make() string {
	var uuid [4]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "RANDBROKE" // should never happen
	}
	return hex.EncodeToString(uuid[:])
}

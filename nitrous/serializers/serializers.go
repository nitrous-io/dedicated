package serializers

import (
	"crypto/md5"
	"encoding/hex"
	"io"
)

func generateGravatarHash(email string) string {
	h := md5.New()
	io.WriteString(h, email)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)
}

package bcrypttest

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestBcrypt(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/bcrypt")
}

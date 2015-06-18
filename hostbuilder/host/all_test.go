package host_test

import (
	"testing"

	"github.com/Sirupsen/logrus"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() { logrus.SetLevel(logrus.PanicLevel) }

func TestHost(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host")
}

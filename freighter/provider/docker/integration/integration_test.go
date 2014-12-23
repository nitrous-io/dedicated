package integration

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/nitrous-io/dedicated/freighter/test"

	"testing"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "provider/docker/integration")
}

var _ = BeforeSuite(func() {
	test.PullImage("tianon/true:latest")
	test.PullImage("busybox:latest")
})

var _ = AfterSuite(func() {
	out, err := exec.Command("docker", "rmi", "tianon/true:latest", "busybox:latest").CombinedOutput()
	Expect(err).To(BeNil(), "failed to remove docker images, output: %s", out)
})

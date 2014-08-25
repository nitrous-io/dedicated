package integration

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "provider/docker/integration")
}

var _ = BeforeSuite(func() {
	err := exec.Command("docker", "pull", "scratch").Run()
	Expect(err).To(BeNil(), "failed to pull scratch:latest image")
})

var _ = AfterSuite(func() {
	err := exec.Command("docker", "rmi", "scratch:latest").Run()
	Expect(err).To(BeNil(), "failed to remove scratch:latest image")
})

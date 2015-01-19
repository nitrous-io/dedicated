package integration

import (
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("StopApp", func() {
	It("kills the NITROUS_CMD process that's running inside the container", func() {
		cmd := "top -b"
		configJSON := fmt.Sprintf(`{ "Env": ["NITROUS_CMD=%s"] }`, cmd)
		// use busybox here because we dont want user to be set
		// su in busybox doesnt support all the options that is required ...
		prov, info, cleanupFn, err := startContainer("busybox:latest", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil(), "failed to start the container")

		// Make sure the process is running inside the container.
		out, _ := exec.Command("docker", "exec", info.ID, "ps").CombinedOutput()
		Expect(string(out)).To(ContainSubstring(cmd), "should start NITROUS_CMD")

		err = prov.StopApp(info.ID)
		Expect(err).To(BeNil(), "failed to stop app")

		// Process should no longer be running.
		out, _ = exec.Command("docker", "exec", info.ID, "ps").CombinedOutput()
		Expect(string(out)).NotTo(ContainSubstring(cmd), "should stop NITROUS_CMD")

		// Stopping when process is not running should not return an error.
		err = prov.StopApp(info.ID)
		Expect(err).To(BeNil(), "failed to stop app")
	})

	It("does not return an error if NITROUS_CMD is not specified", func() {
		prov, info, cleanupFn, err := startContainer("busybox:latest", "")
		defer cleanupFn()
		Expect(err).To(BeNil(), "failed to start the container")

		err = prov.StopApp(info.ID)
		Expect(err).To(BeNil(), "failed to stop app")
	})
})

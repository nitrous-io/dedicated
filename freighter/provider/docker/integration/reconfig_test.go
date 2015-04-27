package integration

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Reconfig", func() {
	It("changes the container volumes", func() {
		var configJSON string

		configJSON = `{
			"Mounts": ["dotfiles:/etc/dotfiles"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		defer removeBackupImages(prov)
		Expect(err).To(BeNil())

		configJSON = `{
			"Mounts": ["shared:/etc/shared"]
		}`
		reconfig, err := prov.Reconfig(info.ID, configJSON)
		defer test.CleanUpContainer(info.ID)

		Expect(err).To(BeNil())
		Expect(reconfig.ID).To(Equal(info.ID), "container ID should not be changed after reconfig")
		Expect(reconfig.Volumes).To(HaveLen(1))
		Expect(reconfig.Volumes).ToNot(HaveKey("dotfiles"))
		Expect(reconfig.Volumes).To(HaveKey("/etc/shared"))
	})

	It("changes the container port bindings", func() {
		var configJSON string

		configJSON = `{
			"PortBindings": ["private:1234:2345", "public:2345:3456"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		defer removeBackupImages(prov)
		Expect(err).To(BeNil())

		configJSON = `{
			"PortBindings": ["private:4567:8910", "public:1111:2222"]
		}`
		reconfig, err := prov.Reconfig(info.ID, configJSON)
		defer test.CleanUpContainer(info.ID)

		extraHostsJson, _ := inspectFieldJSON(info.ID, "HostConfig.ExtraHosts")
		var extraHosts []string
		json.Unmarshal([]byte(extraHostsJson), &extraHosts)

		Expect(err).To(BeNil())
		Expect(reconfig.ID).To(Equal(info.ID), "container ID should not be changed after reconfig")
		assertPort(reconfig.ID, []interface{}{
			fmt.Sprintf("8910/tcp -> %s:4567", reconfig.Gateway),
			"2222/tcp -> 0.0.0.0:1111",
		})
		Expect(extraHosts).To(Equal([]string{fmt.Sprintf("host:%s", reconfig.Gateway)}))
	})

	It("changes the container environment variables", func() {
		var configJSON string

		configJSON = `{
			"Env": ["BRANCH=master","APP_ENV=staging","VERSION=1.0.0"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		defer removeBackupImages(prov)
		Expect(err).To(BeNil())

		configJSON = `{
			"Env": ["BRANCH=release","APP_ENV=production","SECRET=secret"]
		}`
		reconfig, err := prov.Reconfig(info.ID, configJSON)
		defer test.CleanUpContainer(info.ID)

		Expect(err).To(BeNil())
		Expect(reconfig.ID).To(Equal(info.ID), "container ID should not be changed after reconfig")
		assertEnv(reconfig.ID, []string{"BRANCH=release", "APP_ENV=production", "SECRET=secret"}, nil)
	})

	It("changes the container settings", func() {
		var configJSON string

		configJSON = `{
				"PortBindings": ["private:1234:2345", "public:2345:3456"],
				"Mounts": ["dotfiles:/etc/dotfiles"],
				"Env": ["LANG=C","PAGER=more","NITROUS_USERNAME=doofus"]
			}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		defer removeBackupImages(prov)
		Expect(err).To(BeNil())

		// Create some data in existing container so that we can
		// check whether reconfig preserves the data.
		out, err := exec.Command("docker", "exec", info.ID, "bin/sh", "-c", "echo set nocompatible > /root/.vimrc").CombinedOutput()
		Expect(err).To(BeNil(), "docker exec failed, output: %s", out)

		configJSON = `{
				"PortBindings": ["private:4567:8910", "public:1111:2222"],
				"Mounts": ["shared:/etc/shared"],
				"Env": ["BRANCH=master","RAILS_ENV=staging"]
			}`
		reconfig1, err := prov.Reconfig(info.ID, configJSON)
		defer test.CleanUpContainer(info.ID)

		Expect(err).To(BeNil())
		Expect(reconfig1.ID).To(Equal(info.ID), "container ID should not be changed after reconfig")
		assertEnv(reconfig1.ID, []string{"BRANCH=master", "RAILS_ENV=staging"}, nil)
		assertPort(reconfig1.ID, []interface{}{
			fmt.Sprintf("8910/tcp -> %s:4567", reconfig1.Gateway),
			"2222/tcp -> 0.0.0.0:1111",
		})
		Expect(reconfig1.Volumes).To(HaveKey("/etc/shared"))

		out, err = exec.Command("docker", "exec", reconfig1.ID, "cat", "/root/.vimrc").CombinedOutput()
		Expect(err).To(BeNil(), "docker exec failed, output: %s", out)
		Expect(string(out)).To(Equal("set nocompatible\n"), "old container data not migrated")

		configJSON = `{
				"Env": ["PAGER=less"],
				"PortBindings": ["public:4000:4000"]
			}`
		reconfig2, err := prov.Reconfig(reconfig1.ID, configJSON)
		defer test.CleanUpContainer(info.ID)

		Expect(err).To(BeNil())
		assertEnv(reconfig2.ID, []string{"PAGER=less"}, []string{"LANG"})
		assertPort(reconfig2.ID, []interface{}{
			"4000/tcp -> 0.0.0.0:4000",
		})
	})

	It("returns error on failure to complete", func() {
		configJSON := `{
			"PortBindings": ["private:1234:2345", "public:2345:3456"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil())

		_, err = prov.Reconfig(info.ID, `{invalid json}`)
		Expect(err).To(HaveOccurred())
	})

	It("starts back the container when it fails after stopping", func() {
		configJSON := `{
			"PortBindings": ["private:1234:2345", "public:2345:3456"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil())

		reconfigCn, err := prov.Reconfig(info.ID, `{invalid json}`)
		Expect(err).To(HaveOccurred())
		Expect(reconfigCn).NotTo(BeNil())

		Expect(reconfigCn.ID).To(Equal(info.ID))
		Expect(reconfigCn.State).To(Equal(provider.ContainerStateRunning))
		Expect(reconfigCn.ImageID).To(Equal(info.ImageID))

		state, err := inspectField(info.ID, "State.Running")
		Expect(err).To(BeNil())
		Expect(state).To(Equal("true"))
	})

	It("reconfig should not persist auto host port bindings", func() {
		configJSON := `{
			"PortBindings": ["public::1337/tcp"]
		}`

		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil())

		config, err := prov.GetConfig(info.ID)
		Expect(err).To(BeNil())

		// Host port bindings should not be specified in config port bindings
		Expect(config.ConfigPortBindings).To(ConsistOf([]string{"public::1337/tcp"}))

		// Simulate adding one more port binding
		reconfigCn, err := prov.Reconfig(info.ID, `{
			"PortBindings": ["public::1337/tcp", "public::8080/tcp"]
		}`)
		defer test.CleanUpContainer(reconfigCn.ID)
		Expect(err).To(BeNil())

		config2, err := prov.GetConfig(reconfigCn.ID)
		Expect(err).To(BeNil())

		// After reconfig, the old host port bindings should still not be specified in config port bindings
		Expect(config2.ConfigPortBindings).To(ConsistOf([]string{"public::1337/tcp", "public::8080/tcp"}))
	})

	It("changes the container setting but cannot start it when given a config that prevents it from starting", func() {
		configJSON := `{
			"PortBindings": ["private:1234:2345", "public:2345:3456"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil())

		reconfigCn, err := prov.Reconfig(info.ID, `{
			"PortBindings": ["private:1111:2345", "public:1111:3456"]
		}`) // Binding to the same port twice will cause container to not start.
		Expect(err).To(HaveOccurred())
		Expect(reconfigCn).NotTo(BeNil())
		defer test.CleanUpContainer(reconfigCn.ID)

		state, err := inspectField(reconfigCn.ID, "State.Running")
		Expect(err).To(BeNil())
		Expect(state).To(Equal("false"))

		config, err := prov.GetConfig(reconfigCn.ID)
		Expect(err).To(BeNil())

		Expect(config.ConfigPortBindings).To(ConsistOf([]string{"private:1111:2345/tcp", "public:1111:3456/tcp"}))
	})

	It("removes built-in volumes when reconfig mount options do not include them", func() {
		prov, info, cleanupFn, err := startContainer("busybox-volumes", "")
		defer cleanupFn()
		Expect(err).To(BeNil())
		Expect(info.Volumes).To(HaveKey("/var/lib/docker"))
		Expect(info.Volumes).To(HaveKey("/var/lib/extra"))

		reconfigCn, _ := prov.Reconfig(info.ID, `{
			"Mounts": ["dotfiles:/etc/dotfiles"]
		}`) // extra built-in volumes have beenr removed
		Expect(reconfigCn.Volumes).ToNot(HaveKey("/var/lib/docker"))
		Expect(reconfigCn.Volumes).ToNot(HaveKey("/var/lib/extra"))
	})
})

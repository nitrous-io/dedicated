package integration

import (
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Reconfig", func() {
	It("changes the container volumes", func() {
		var configJSON string

		configJSON = `{
			"Mounts": ["dotfiles:/etc/dotfiles"]
		}:`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		defer removeBackupImages(prov)
		Expect(err).To(BeNil())

		configJSON = `{
			"Mounts": ["shared:/etc/shared"]
		}`
		reconfig, err := prov.NewReconfig(info.ID, configJSON)
		if reconfig != nil {
			defer test.CleanUpContainer(info.ID)
		}

		Expect(err).To(BeNil())
		Expect(reconfig.ID).To(Equal(info.ID), "container ID should not be changed after reconfig")
		Expect(reconfig.Volumes).To(HaveLen(1))
		Expect(reconfig.Volumes).ToNot(HaveKey("dotfiles"))
		Expect(reconfig.Volumes).To(HaveKey("/etc/shared"))
	})
	/*
		It("changes the container settings", func() {
			var configJSON string

			configJSON = `{
				"PortBindings": ["private:1234:2345", "public:2345:3456"],
				"Mounts": ["dotfiles:/etc/dotfiles"],
				"Env": ["LANG=C","PAGER=more","NITROUS_USERNAME=doofus"]
			}:`
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
				"Mounts": ["/opt/shared:/etc/shared"],
				"Env": ["BRANCH=master","RAILS_ENV=staging"]
			}`
			reconfig1, err := prov.NewReconfig(info.ID, configJSON)
			if reconfig1 != nil {
				defer test.CleanUpContainer(info.ID)
			}
			Expect(err).To(BeNil())
			Expect(reconfig1.ID).To(Equal(info.ID), "container ID should not be changed after reconfig")
			assertEnv(reconfig1.ID, []string{"BRANCH=master", "RAILS_ENV=staging"}, nil)
			assertPort(reconfig1.ID, []interface{}{
				fmt.Sprintf("4567/tcp -> %s:8910", reconfig1.Gateway),
				"1111/tcp -> 0.0.0.0:2222",
			})

			out, err = exec.Command("docker", "exec", reconfig1.ID, "cat", "/root/.vimrc").CombinedOutput()
			Expect(err).To(BeNil(), "docker exec failed, output: %s", out)
			Expect(string(out)).To(Equal("set nocompatible\n"), "old container data not migrated")

			configJSON = `{
				"Env": ["PAGER=less"],
				"PortBindings": ["public:4000:4000"]
			}`
			reconfig2, err := prov.NewReconfig(reconfig1.ID, configJSON)
			if reconfig2 != nil {
				defer test.CleanUpContainer(info.ID)
			}
			Expect(err).To(BeNil())
			assertEnv(reconfig2.ID, []string{"PAGER=less"}, []string{"LANG"})
			assertPort(reconfig2.ID, []interface{}{
				"4000/tcp -> 0.0.0.0:4000",
			})
		})
	*/
})

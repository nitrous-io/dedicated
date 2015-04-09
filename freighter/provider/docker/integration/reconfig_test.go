package integration

import (
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Reconfig", func() {
	It("changes the container id and settings", func() {
		var configJSON string

		configJSON = `{
			"PortBindings": ["private:1234:2345", "public:2345:3456"]
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
			"Env": ["LANG=C","PAGER=more","NITROUS_USERNAME=doofus"],
			"PortBindings": ["private:65000:3000", "public:9999:9999"]
		}`
		reconfig1, err := prov.Reconfig(info.ID, configJSON)
		if reconfig1 != nil {
			// id will change, default cleanup won't catch
			// if the second reconfig succeeds, cleanUp of the
			// first container not necessary
			defer func() {
				out, _ := exec.Command("docker", "inspect", reconfig1.ID).CombinedOutput()
				if !strings.Contains(string(out), "No such image or container") {
					test.CleanUpContainer(reconfig1.ID)
				}
			}()
		}
		Expect(err).To(BeNil())
		Expect(reconfig1.ID).NotTo(Equal(info.ID), "container ID should be changed after reconfig")
		assertEnv(reconfig1.ID, []string{"LANG=C", "PAGER=more", "NITROUS_USERNAME=doofus"}, nil)
		assertPort(reconfig1.ID, []interface{}{
			fmt.Sprintf("3000/tcp -> %s:65000", reconfig1.Gateway),
			"9999/tcp -> 0.0.0.0:9999",
		})

		out, err = exec.Command("docker", "exec", reconfig1.ID, "cat", "/root/.vimrc").CombinedOutput()
		Expect(err).To(BeNil(), "docker exec failed, output: %s", out)
		Expect(string(out)).To(Equal("set nocompatible\n"), "old container data not migrated")

		configJSON = `{
			"Env": ["PAGER=less"],
			"PortBindings": ["public:4000:4000"]
		}`
		reconfig2, err := prov.Reconfig(reconfig1.ID, configJSON)
		if reconfig2 != nil {
			defer test.CleanUpContainer(reconfig2.ID)
		}
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

	It("starts back the original container when it fails after stopping but before terminating the original container", func() {
		configJSON := `{
			"PortBindings": ["private:1234:2345", "public:2345:3456"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil())

		reconfigCn, err := prov.Reconfig(info.ID, `{invalid json}`)
		Expect(err).To(HaveOccurred())
		Expect(reconfigCn).NotTo(BeNil())

		Expect(reconfigCn.ID).To(Equal(info.ID)) // Container ID should not have changed since it was only stopped.
		Expect(reconfigCn.State).To(Equal(provider.ContainerStateRunning))
		Expect(reconfigCn.ImageID).To(Equal(info.ImageID))

		state, err := inspectField(info.ID, "State.Running")
		Expect(err).To(BeNil())
		Expect(state).To(Equal("true"))

		imageID, err := inspectField(info.ID, "Image")
		Expect(err).To(BeNil())
		Expect(imageID).To(Equal(info.ImageID))
	})

	It("creates a new container from the original container's image when it fails after terminating the original container", func() {
		configJSON := `{
			"PortBindings": ["private:1234:2345", "public:2345:3456"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil())

		reconfigCn, err := prov.Reconfig(info.ID, `{
			"PortBindings": ["private:666666:80"]
		}`) // A host port out of range will cause container to fail when creating.
		Expect(err).To(HaveOccurred())
		Expect(reconfigCn).NotTo(BeNil())

		Expect(reconfigCn.ID).NotTo(Equal(info.ID))

		state, err := inspectField(reconfigCn.ID, "State.Running")
		Expect(err).To(BeNil())
		Expect(state).To(Equal("true"))

		imageID, err := inspectField(reconfigCn.ID, "Image")
		Expect(err).To(BeNil())
		Expect(imageID).NotTo(Equal(info.ImageID)) // Image should be a different one.

		// Container should use backup image.
		Expect(reconfigCn.Image).To(MatchRegexp(fmt.Sprintf(`^%s-backup-\d+$`, info.Name)))
	})

	// Marked as pending because I cannot figure out a way to make container
	// termination fail.
	PIt("creates a new container from the original container's image when it runs into an error while terminating the original container, but the container was actually terminated", func() {
		configJSON := `{
			"PortBindings": ["private:1234:2345", "public:2345:3456"]
		}`
		prov, info, cleanupFn, err := startContainer("busybox", configJSON)
		defer cleanupFn()
		Expect(err).To(BeNil())

		// FIXME This below doesn't actually work - the container is still
		// terminated successfully.
		//
		// Make container fail to terminate by bind-mounting a read-only file
		// in its diff directory.
		roPath := path.Join("/var/lib/docker/aufs/diff", info.ID, "readonly-file")
		err = exec.Command("sudo", "touch", roPath).Run()
		Expect(err).To(BeNil())
		err = exec.Command("sudo", "mount", "--bind", roPath, roPath).Run()
		Expect(err).To(BeNil())
		err = exec.Command("sudo", "mount", "-o", "remount,bind,ro", roPath).Run()
		Expect(err).To(BeNil())
		err = exec.Command("sudo", "ls", "-alh", roPath).Run()

		reconfigCn, err := prov.Reconfig(info.ID, `{}`)
		Expect(err).To(HaveOccurred()) // FIXME Test currently fails here (i.e. err is nil).
		Expect(reconfigCn).NotTo(BeNil())

		Expect(reconfigCn.ID).NotTo(Equal(info.ID))

		state, err := inspectField(reconfigCn.ID, "State.Running")
		Expect(err).To(BeNil())
		Expect(state).To(Equal("true"))

		imageID, err := inspectField(reconfigCn.ID, "Image")
		Expect(err).To(BeNil())
		Expect(imageID).NotTo(Equal(info.ImageID)) // Image should be a different one.

		// Container should use backup image.
		Expect(reconfigCn.Image).To(MatchRegexp(fmt.Sprintf(`^%s-backup-\d+$`, info.Name)))
	})

	It("creates a new container but cannot start it when given a config that prevents it from starting", func() {
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

		Expect(reconfigCn.ID).NotTo(Equal(info.ID))
		defer test.CleanUpContainer(reconfigCn.ID)

		state, err := inspectField(reconfigCn.ID, "State.Running")
		Expect(err).To(BeNil())
		Expect(state).To(Equal("false"))

		imageID, err := inspectField(reconfigCn.ID, "Image")
		Expect(err).To(BeNil())
		Expect(imageID).NotTo(Equal(info.ImageID)) // Image should be a different one.

		// Container should use reconfig image.
		Expect(reconfigCn.Image).To(MatchRegexp(fmt.Sprintf(`^%s-reconfig-\d+$`, info.Name)))
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

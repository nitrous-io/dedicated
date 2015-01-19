package integration

import (
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ListImages", func() {
	It("should fetch basic images info", func() {
		datadir, _ := ioutil.TempDir("", "freightertest")
		defer os.RemoveAll(datadir)
		prov, _ := docker.NewDockerProvider(&util.Logger{Level: util.LogLevelQuiet}, datadir)

		imageList, err := prov.ListImages()
		Expect(err).To(BeNil(), "failed to list images")

		var image *provider.ImageInfo
		for _, img := range imageList {
			if img.Name == "nitrous/busybox" {
				image = img
				break
			}
		}
		Expect(image).ToNot(BeNil(), "failed to find nitrous/busybox")
		Expect(image.Name).To(Equal("nitrous/busybox"))
		Expect(image.Tag).To(Equal("latest"))
		Expect(image.VirtualSize > 0).To(BeTrue(), "invalid size")
		Expect(image.CreatedAt > 0).To(BeTrue(), "invalid CreatedAt")
	})

	It("should break them out into separate entries when the same image is under multiple repositories", func() {
		datadir, _ := ioutil.TempDir("", "freightertest")
		defer os.RemoveAll(datadir)
		prov, _ := docker.NewDockerProvider(&util.Logger{Level: util.LogLevelQuiet}, datadir)

		cmd := exec.Command("docker", "tag", "busybox:latest", "busybox:v1")
		Expect(cmd.Run()).To(BeNil(), "failed to tag busybox:latest")
		defer exec.Command("docker", "rmi", "busybox:v1").Run()

		images, err := prov.ListImages()
		Expect(err).To(BeNil(), "failed to list images")

		var i1, i2 *provider.ImageInfo
		for _, i := range images {
			if i.Name == "busybox" && i.Tag == "latest" {
				i1 = i
			}
			if i.Name == "busybox" && i.Tag == "v1" {
				i2 = i
			}
		}
		Expect(i1).NotTo(BeNil())
		Expect(i2).NotTo(BeNil())
		Expect(i1.ID).To(Equal(i2.ID))
	})
})

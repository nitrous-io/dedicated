package docker_test

import (
	"os"
	"path/filepath"

	"github.com/nitrous-io/freighter/provider/docker"
	. "github.com/nitrous-io/freighter/test"
	"github.com/nitrous-io/freighter/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider/docker volumes", func() {
	var (
		prov      *docker.DockerProvider
		namespace = "freightertest"
		datadir   string
	)

	BeforeEach(func() {
		var err error

		datadir, err = TempDir()
		Expect(err).To(BeNil())
		logger := &util.Logger{Level: util.LogLevelQuiet}
		prov, err = docker.NewDockerProvider(logger, datadir, namespace)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		os.RemoveAll(datadir)
	})

	Describe(".CreateVolume()", func() {
		It("creates a volume", func() {
			slug := "my-data"
			err := prov.CreateVolume("my-data")
			Expect(err).To(BeNil())
			_, err = os.Stat(prov.VolumesDir(slug))
			Expect(err).To(BeNil())
		})

		It("errs on missing slug", func() {
			Expect(prov.CreateVolume("")).To(MatchError(docker.ErrMissingVolumeSlug))
		})

		It("errs on invalid slug", func() {
			for _, s := range []string{"../../pwned",
				"sub/dir", "justbad!", "$(echo nice try)"} {
				Expect(prov.CreateVolume(s)).To(MatchError(docker.ErrBadVolumeSlugFormat))
			}
		})

		Context("on existing directory", func() {
			var (
				slug = "existing"
				path string
			)
			BeforeEach(func() {
				path = prov.VolumesDir(slug)
				err := os.MkdirAll(path, 0755)
				Expect(err).To(BeNil())
			})
			AfterEach(func() { err := os.RemoveAll(path); Expect(err).To(BeNil()) })

			It("returns error", func() {
				Expect(prov.CreateVolume(slug)).To(MatchError(docker.ErrVolumeExists))
			})
		})
	})

	Describe(".ListVolumes()", func() {
		var (
			volBase     string
			dirs, files []string
		)

		BeforeEach(func() {
			volBase = prov.VolumesDir("")
			dirs = []string{"dotfiles", "Shared", "mpd-albums"}
			files = []string{"README.txt", "config.json"}
			for _, d := range dirs {
				err := os.MkdirAll(filepath.Join(volBase, d), 0755)
				Expect(err).To(BeNil())
			}
			for _, f := range files {
				fi, err := os.Create(filepath.Join(volBase, f))
				Expect(err).To(BeNil())
				fi.Close()
			}
		})

		It("returns a list of volumes", func() {
			vols, err := prov.ListVolumes()
			Expect(err).To(BeNil())
			Expect(vols).To(Equal([]string{"Shared", "dotfiles", "mpd-albums"}))
		})
	})

	Describe(".RemoveVolume()", func() {
		var (
			volBase     string
			dirs, files []string
		)

		BeforeEach(func() {
			volBase = prov.VolumesDir("")
			dirs = []string{"dotfiles", "Shared", "mpd-albums"}
			for _, d := range dirs {
				err := os.MkdirAll(filepath.Join(volBase, d), 0755)
				Expect(err).To(BeNil())
			}
			for _, f := range files {
				fi, err := os.Create(filepath.Join(volBase, f))
				Expect(err).To(BeNil())
				fi.Close()
			}
		})

		It("removes volume path", func() {
			err := prov.RemoveVolume("Shared")
			Expect(err).To(BeNil())

			_, err = os.Stat(filepath.Join(volBase, "Shared"))
			Expect(os.IsNotExist(err)).To(BeTrue())

			vols, err := prov.ListVolumes()
			Expect(err).To(BeNil())
			Expect(vols).To(Equal([]string{"dotfiles", "mpd-albums"}))
		})

		It("returns error when slug is not found", func() {
			err := prov.RemoveVolume("nosuch")
			Expect(err).To(MatchError(docker.ErrVolumeNotFound))
		})
	})

	Describe(".VolumesDir()", func() {
		It("returns parent dir of volumes when given empty string", func() {
			Expect(prov.VolumesDir("")).To(Equal(filepath.Join(datadir, "volumes")))
		})

		It("returns volume dir when given slug", func() {
			Expect(prov.VolumesDir("shared")).To(Equal(filepath.Join(datadir, "volumes", "shared")))
		})
	})
})

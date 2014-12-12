package docker_test

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	. "github.com/nitrous-io/dedicated/freighter/test"
	"github.com/nitrous-io/dedicated/freighter/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider/docker volumes", func() {
	var (
		prov    *docker.DockerProvider
		datadir string
	)

	BeforeEach(func() {
		var err error

		datadir, err = TempDir()
		Expect(err).To(BeNil())
		logger := &util.Logger{Level: util.LogLevelQuiet}
		prov, err = docker.NewDockerProvider(logger, datadir)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		os.RemoveAll(datadir)
	})

	Describe(".CreateVolume()", func() {
		It("creates a volume", func() {
			slug := "my-data"
			vi, err := prov.CreateVolume("my-data")
			Expect(err).To(BeNil())
			_, err = os.Stat(prov.VolumesDir(slug))
			Expect(err).To(BeNil())
			Expect(vi.Slug).To(Equal("my-data"))
			Expect(vi.Size).To(Equal(uint64(0)))
		})

		It("errs on missing slug", func() {
			_, err := prov.CreateVolume("")
			Expect(err).To(MatchError(docker.ErrMissingVolumeSlug))
		})

		It("errs on invalid slug", func() {
			for _, s := range []string{"../../pwned",
				"sub/dir", "justbad!", "$(echo nice try)"} {
				_, err := prov.CreateVolume(s)
				Expect(err).To(MatchError(docker.ErrBadVolumeSlugFormat))
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
				_, err := prov.CreateVolume(slug)
				Expect(err).To(MatchError(docker.ErrVolumeExists))
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

	Describe(".GetVolumeInfo()", func() {
		var (
			volBase     string
			fileContent = []byte("take up space")
		)

		BeforeEach(func() {
			volBase = prov.VolumesDir("")
			vpath := filepath.Join(volBase, "backups")
			fpath := filepath.Join(vpath, "very", "deeply", "nested")
			err := os.MkdirAll(fpath, 0755)
			Expect(err).To(BeNil())
			err = ioutil.WriteFile(filepath.Join(fpath, "inner-production.psql"), fileContent, 0644)
			Expect(err).To(BeNil())
			err = ioutil.WriteFile(filepath.Join(fpath, "outer-production.psql"), fileContent, 0644)
			Expect(err).To(BeNil())
		})

		It("returns volume info if exists", func() {
			fileSize := uint64(len(fileContent))
			vi, err := prov.GetVolumeInfo("backups")
			Expect(err).To(BeNil())
			Expect(vi.Slug).To(Equal("backups"))
			Expect(vi.Size).To(Equal(fileSize * 2))
		})

		It("returns zero size on new volume", func() {
			vpath := filepath.Join(volBase, "emptydir")
			err := os.MkdirAll(vpath, 0755)
			vi, err := prov.GetVolumeInfo("emptydir")
			Expect(err).To(BeNil())
			Expect(vi.Size).To(BeZero())
		})

		It("returns err if volume doesn't exist", func() {
			vi, err := prov.GetVolumeInfo("nosuch")
			Expect(vi).To(BeNil())
			Expect(err).To(MatchError(docker.ErrVolumeNotFound))
		})
	})

	Describe(".ExportVolume()", func() {
		BeforeEach(func() {
			base := prov.VolumesDir("")
			err := os.MkdirAll(filepath.Join(base, "exported"), 0755)
			Expect(err).To(BeNil())
			ioutil.WriteFile(filepath.Join(base, "exported", "file.txt"), []byte("data"), 0644)
		})

		It("exports the volume as a .tar", func() {
			rd, wr := io.Pipe()
			err := prov.ExportVolume("exported", wr)
			Expect(err).To(BeNil())

			tr := tar.NewReader(rd)

			hdr, err := tr.Next()
			Expect(err).To(BeNil())
			Expect(hdr.Name).To(Equal("./"))

			hdr, err = tr.Next()
			Expect(err).To(BeNil())
			Expect(hdr.Name).To(Equal("./file.txt"))
			data := make([]byte, 4)
			n, err := tr.Read(data)
			Expect(err).To(BeNil())
			Expect(n).To(Equal(4))
			Expect(data).To(Equal([]byte("data")))
		})

		It("returns error when slug is not found", func() {
			_, wr := io.Pipe()
			err := prov.ExportVolume("nosuch", wr)
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

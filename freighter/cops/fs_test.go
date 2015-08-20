package cops

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "cops/cfs")
}

var _ = Describe("mkdir", func() {
	It("creates a directory with 755 permission", func() {
		root, _ := ioutil.TempDir("/tmp", "cfs-mkdir")
		defer os.RemoveAll(root)
		p := filepath.Join(root, "testdir")
		fs := &FS{}
		err := fs.mkdir(p)
		Expect(err).To(BeNil(), "unexpected error")

		stat, _ := os.Stat(p)
		Expect(stat.IsDir()).To(BeTrue(), "expect a directory")
		Expect(stat.Mode().String()).To(Equal("drwxr-xr-x"), "unexpected permission")
	})

	It("sets the owner to the fs uid/gid", func() {
		root, _ := ioutil.TempDir("/tmp", "cfs-mkdir")
		defer os.RemoveAll(root)
		os.Chown(root, 1234, 123)

		p := filepath.Join(root, "testdir")
		fs := &FS{uid: 1000, gid: 1000}
		err := fs.mkdir(p)
		Expect(err).To(BeNil(), "unexpected error")

		stat, _ := os.Stat(p)
		s, _ := stat.Sys().(*syscall.Stat_t)
		Expect(int(s.Uid)).To(Equal(1000), "unexpected uid")
		Expect(int(s.Gid)).To(Equal(1000), "unexpected gid")
	})
})

var _ = Describe("openFile", func() {
	It("opens a file with 644 attribute and correct uid/gid", func() {
		root, _ := ioutil.TempDir("/tmp", "cfs-mkdir")
		defer os.RemoveAll(root)
		os.Chown(root, 1234, 123)

		fs := &FS{uid: 1000, gid: 1000}
		p := filepath.Join(root, "testfile")
		f, err := fs.openFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC)
		Expect(err).To(BeNil(), "unexpected error")

		stat, _ := f.Stat()
		Expect(stat.Mode()).To(Equal(os.FileMode(0644)))

		s, _ := stat.Sys().(*syscall.Stat_t)
		Expect(int(s.Uid)).To(Equal(1000), "unexpected uid")
		Expect(int(s.Gid)).To(Equal(1000), "unexpected gid")
	})
})

var _ = Describe("readDir", func() {
	It("returns files in the directory", func() {
		tempdir, _ := ioutil.TempDir("/tmp", "test")
		defer os.RemoveAll(tempdir)

		// Test for regular directory.
		err := os.MkdirAll(filepath.Join(tempdir, "subdir"), 0755)
		Expect(err).To(BeNil())

		entries, err := readDir(tempdir)
		Expect(err).To(BeNil())
		Expect(entries).To(HaveLen(1))
		entry := entries[0].([]interface{})
		Expect(entry[0]).To(Equal("subdir")) // Name.
		Expect(entry[1]).To(Equal(true))     // Is this a directory?
		// 3rd item is file size, which we do not care about.
		Expect(entry[3]).To(Equal(filepath.Join(tempdir, "subdir"))) // Real path.

		// Test for regular file.
		f, err := os.Create(filepath.Join(tempdir, "subdir", "a.go"))
		Expect(err).To(BeNil())
		defer f.Close()

		entries, err = readDir(filepath.Join(tempdir, "subdir"))
		Expect(err).To(BeNil())
		Expect(entries).To(HaveLen(1))
		entry = entries[0].([]interface{})
		Expect(entry[0]).To(Equal("a.go"))
		Expect(entry[1]).To(Equal(false))
		Expect(entry[3]).To(Equal(filepath.Join(tempdir, "subdir", "a.go")))
	})

	It("handles symlinks by properly including their real paths", func() {
		tempdir, _ := ioutil.TempDir("/tmp", "test")
		defer os.RemoveAll(tempdir)

		err := os.MkdirAll(filepath.Join(tempdir, "subdir"), 0755)
		Expect(err).To(BeNil())

		f1, err := os.Create(filepath.Join(tempdir, "doc.go"))
		Expect(err).To(BeNil())
		defer f1.Close()

		err = os.Symlink(filepath.Join(tempdir, "doc.go"), filepath.Join(tempdir, "subdir", "symlink.go"))
		Expect(err).To(BeNil())

		entries, err := readDir(filepath.Join(tempdir, "subdir"))
		Expect(err).To(BeNil())
		Expect(entries).To(HaveLen(1))
		entry := entries[0].([]interface{})
		Expect(entry[0]).To(Equal("symlink.go"))
		Expect(entry[1]).To(Equal(false))
		Expect(entry[3]).To(Equal(filepath.Join(tempdir, "doc.go")))
	})

	It("handles files within a symlink to a directory by properly including their real paths", func() {
		tempdir, _ := ioutil.TempDir("/tmp", "test")
		defer os.RemoveAll(tempdir)

		err := os.MkdirAll(filepath.Join(tempdir, "subdir"), 0755)
		Expect(err).To(BeNil())

		f, err := os.Create(filepath.Join(tempdir, "subdir", "doc.go"))
		Expect(err).To(BeNil())
		defer f.Close()

		err = os.Symlink(filepath.Join(tempdir, "subdir"), filepath.Join(tempdir, "subdir-symlink"))
		Expect(err).To(BeNil())

		entries, err := readDir(filepath.Join(tempdir, "subdir-symlink"))
		Expect(err).To(BeNil())
		Expect(entries).To(HaveLen(1))
		entry := entries[0].([]interface{})
		Expect(entry[0]).To(Equal("doc.go"))
		Expect(entry[1]).To(Equal(false))
		Expect(entry[3]).To(Equal(filepath.Join(tempdir, "subdir", "doc.go")))
	})
})

package cops

import (
	"io/ioutil"
	"os"
	"path/filepath"
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

	It("sets the owner to the parent directory's owner", func() {
		root, _ := ioutil.TempDir("/tmp", "cfs-mkdir")
		defer os.RemoveAll(root)
		os.Chown(root, 1234, 123)

		p := filepath.Join(root, "testdir")
		fs := &FS{}
		err := fs.mkdir(p)
		Expect(err).To(BeNil(), "unexpected error")

		stat, _ := os.Stat(p)
		uid, gid, _ := owner(stat)
		Expect(uid).To(Equal(1234), "unexpected uid")
		Expect(gid).To(Equal(123), "unexpected gid")
	})
})

var _ = Describe("openFile", func() {
	It("opens a file with 644 attribute", func() {
		root, _ := ioutil.TempDir("/tmp", "cfs-mkdir")
		defer os.RemoveAll(root)
		os.Chown(root, 1234, 123)

		p := filepath.Join(root, "testfile")
		f, err := openFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC)
		Expect(err).To(BeNil(), "unexpected error")

		stat, _ := f.Stat()
		Expect(stat.Mode()).To(Equal(os.FileMode(0644)))
	})
})

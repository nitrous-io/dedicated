package cfs

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
})

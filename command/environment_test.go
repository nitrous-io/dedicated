package command_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/nitrous-io/nitro/command"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEnvironment(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "environment")
}

var _ = Describe("environment", func() {
	var originalEnvironment *command.Environment

	BeforeEach(func() {
		originalEnvironment = command.ConfigureEnvironment()
	})

	AfterEach(func() {
		os.Setenv("NITRO_CONFIG_PATH", originalEnvironment.ConfigPath)
		os.Setenv("NITRO_API_URL", originalEnvironment.APIURL)
	})

	Context("APIURL", func() {
		It("sets from environment", func() {
			os.Setenv("NITRO_API_URL", "http://example.com")
			Expect(command.ConfigureEnvironment().APIURL).To(Equal("http://example.com"))
		})

		It("defaults to ~/.nitrocfg", func() {
			os.Setenv("NITRO_API_URL", "")
			Expect(command.ConfigureEnvironment().APIURL).To(Equal("http://127.0.0.1:3000"))
		})
	})

	Context("ConfigPath", func() {
		It("sets from environment", func() {
			os.Setenv("NITRO_CONFIG_PATH", "/tmp/myconf")
			Expect(command.ConfigureEnvironment().ConfigPath).To(Equal("/tmp/myconf"))
		})

		It("defaults to ~/.nitrocfg", func() {
			os.Setenv("NITRO_CONFIG_PATH", "")
			Expect(command.ConfigureEnvironment().ConfigPath).To(Equal(filepath.Join(os.Getenv("HOME"), ".nitrocfg")))
		})
	})

	createConfig := func(conf map[string]command.AuthConfig) *os.File {
		f, err := ioutil.TempFile("", "nitrocfg")
		Expect(err).To(BeNil())
		os.Setenv("NITRO_CONFIG_PATH", f.Name())
		s, err := json.Marshal(conf)
		Expect(err).To(BeNil())
		f.Write(s)
		f.Close()
		return f
	}

	Context("Config", func() {
		It("sets from file", func() {
			f := createConfig(map[string]command.AuthConfig{
				command.DefaultAPIURL: {"someuser", "a_token"},
			})
			defer os.Remove(f.Name())

			auths := command.ConfigureEnvironment().Config.Auths
			Expect(auths).To(HaveLen(1))
			authConfig, ok := auths[command.DefaultAPIURL]
			Expect(ok).To(BeTrue())
			Expect(authConfig.Username).To(Equal("someuser"))
			Expect(authConfig.Token).To(Equal("a_token"))
		})

		It("defaults to empty", func() {
			os.Setenv("NITRO_CONFIG_PATH", "/tmp/myconf")
			Expect(command.ConfigureEnvironment().Config.Auths).To(BeEmpty())
		})
	})

	Context("AuthToken", func() {
		It("sets from file", func() {
			f := createConfig(map[string]command.AuthConfig{
				command.DefaultAPIURL: {"someuser", "a_token"},
			})
			defer os.Remove(f.Name())

			token := command.ConfigureEnvironment().AuthToken
			Expect(token).To(Equal("a_token"))
		})

		It("defaults to empty", func() {
			os.Setenv("NITRO_CONFIG_PATH", "/tmp/myconf")
			Expect(command.ConfigureEnvironment().AuthToken).To(BeEmpty())
		})
	})
})

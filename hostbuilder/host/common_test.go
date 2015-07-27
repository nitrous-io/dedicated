package host_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/host/dummy"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
)

type dumbLogger struct{}

func (l *dumbLogger) Printf(format string, v ...interface{}) {}
func (l *dumbLogger) MetricsLabel(prefix string) string      { return "" }

type dumbExecutor struct {
	cmds         []string
	uploaded     map[string]string
	nextOutput   []byte
	nextError    error
	nextDuration time.Duration
	isClosed     bool
}

func (e *dumbExecutor) CombinedOutput(cmd string) ([]byte, error) {
	e.cmds = append(e.cmds, cmd)
	time.Sleep(e.nextDuration)
	return e.nextOutput, e.nextError
}

func (e *dumbExecutor) Execute(cmd string) error {
	e.cmds = append(e.cmds, cmd)
	time.Sleep(e.nextDuration)
	return e.nextError
}

func (e *dumbExecutor) Close() {
}

func (e *dumbExecutor) Upload(path string, input io.Reader) error {
	if e.uploaded == nil {
		e.uploaded = make(map[string]string)
	}
	time.Sleep(e.nextDuration)
	b := bytes.NewBuffer(nil)
	io.Copy(b, input)
	e.uploaded[path] = b.String()
	return e.nextError
}

var _ = Describe("NewSettings", func() {
	It("populates settings with default values", func() {
		settings := host.NewSettings(
			&host.TLSConfig{CA: "ca", Cert: "cert", Key: "key"},
			&host.AptConfig{AptUsername: "distro", AptPassword: "passwd", AptLocation: "master", AptDist: "precise"},
			"/home/action",
			"http://google.com",
			"secret",
			"0.0.1",
			[]string{"pubkey1", "pubkey2"},
		)

		Expect(settings.TLSConfig).To(Equal(&host.TLSConfig{CA: "ca", Cert: "cert", Key: "key"}))
		Expect(settings.AptConfig).To(Equal(&host.AptConfig{AptUsername: "distro", AptPassword: "passwd", AptLocation: "master", AptDist: "precise"}))
		Expect(settings.Config.NotifyURL).To(Equal("http://google.com"))
		Expect(settings.Config.TLSVerify).To(BeTrue())
		Expect(settings.Home).To(Equal("/home/action"))
		Expect(settings.Secret).To(Equal("secret"))
		Expect(settings.Version).To(Equal("0.0.1"))
		Expect(settings.PublicKeys).To(ConsistOf("pubkey1", "pubkey2"))
	})
})

var _ = Describe("WriteSettings", func() {
	It("uploads settings to the remote server", func() {
		e := &dumbExecutor{}
		ctx := host.NewContext(context.Background(), &dummy.Host{}, &dumbLogger{}, ioutil.Discard)
		ctx.Executor = e

		err := host.WriteSettings(ctx)
		Expect(err).To(BeNil())

		Expect(e.cmds[0]).To(Equal("sudo mkdir -p /etc/freighterd"))

		Expect(e.cmds[1]).To(Equal(`echo -e "pubkey1\npubkey2" >> /home/action/.ssh/authorized_keys`))

		uploads := [][3]string{
			// destination, contents, equality ("" = test contents for substring)
			{"/etc/freighterd/ca.crt", "ca", "true"},
			{"/etc/freighterd/cert.crt", "cert", "true"},
			{"/etc/freighterd/key.crt", "key", "true"},
			{"/etc/freighterd/secret", "secret", "true"},
			{"/etc/freighterd/config.json", "http://google.com", ""},
			{host.SourceLocation, "deb https://distro:passwd@master precise main", ""},
		}

		for _, v := range uploads {
			dest, contents, equality := v[0], v[1], v[2] != ""
			tmpDest := filepath.Join("/tmp", hex.EncodeToString([]byte(dest)))
			if equality {
				Expect(e.uploaded[tmpDest]).To(Equal(contents), "expected %s (tempfile %s) to have content", dest, tmpDest)
			} else {
				Expect(e.uploaded[tmpDest]).To(ContainSubstring(contents), "expected %s (tempfile %s) to contain content", dest, tmpDest)
			}
			Expect(e.cmds).To(ContainElement(fmt.Sprintf(`sudo mv %s "%s"`, tmpDest, dest)))
		}
	})

	It("cancels early when the operation times out", func() {
		ctx := host.NewContext(context.Background(), &dummy.Host{}, &dumbLogger{}, ioutil.Discard)
		ctx.Executor = &dumbExecutor{nextDuration: time.Second}

		start := time.Now()
		err := ctx.Run(host.Step{
			Func:    host.WriteSettings,
			Timeout: time.Millisecond,
		})

		Expect(time.Since(start) < time.Second).To(BeTrue(), "failed to timed out. Taken %v", time.Since(start))
		Expect(err).To(Equal(context.DeadlineExceeded))
	})
})

var _ = Describe("RestartServices", func() {
	It("restarts the freighterd service", func() {
		e := &dumbExecutor{}
		ctx := host.NewContext(context.Background(), &dummy.Host{}, &dumbLogger{}, ioutil.Discard)
		ctx.Executor = e

		err := host.RestartServices(ctx)
		Expect(err).To(BeNil())
		Expect(e.cmds[0]).To(Equal("sudo service freighterd restart"))
	})

	It("cancels early when the operation times out", func() {
		ctx := host.NewContext(context.Background(), &dummy.Host{}, &dumbLogger{}, ioutil.Discard)
		ctx.Executor = &dumbExecutor{nextDuration: time.Second}

		start := time.Now()
		err := ctx.Run(host.Step{
			Func:    host.RestartServices,
			Timeout: time.Millisecond,
		})

		Expect(time.Since(start) < time.Second).To(BeTrue(), "failed to timed out. Taken %v", time.Since(start))
		Expect(err).To(Equal(context.DeadlineExceeded))
	})
})

var _ = Describe("UpdateSoftware", func() {
	It("update the freighterd service", func() {
		e := &dumbExecutor{}
		ctx := host.NewContext(context.Background(), &dummy.Host{Version: "0.0.1"}, &dumbLogger{}, ioutil.Discard)
		ctx.Executor = e

		err := host.UpdateSoftware(ctx)
		Expect(err).To(BeNil())
		parts := []string{
			"DEBIAN_FRONTEND=noninteractive sudo apt-get update",
			"-o APT::List-Cleanup=0",
			`-o Dir::Etc::sourceparts="/dev/null"`,
			`-o Dir::Etc::sourcelist="sources.list.d/nitrousio.list"`,
			`&&`,
			"DEBIAN_FRONTEND=noninteractive sudo apt-get install -y --force-yes",
			`-o Dpkg::Options::="--force-confdef"`, // avoid prompt
			`-o Dpkg::Options::="--force-confold"`, // do not modify existing config file
		}
		Expect(e.cmds[0]).To(ContainSubstring(strings.Join(parts, " ")))
	})

	It("fails when specifying version results in error", func() {
		ctx := host.NewContext(context.Background(), &dummy.Host{Version: "0.0.1"}, &dumbLogger{}, ioutil.Discard)
		ctx.Executor = &dumbExecutor{
			nextError: errors.New("invalid version"),
		}
		err := host.UpdateSoftware(ctx)
		Expect(err).ToNot(BeNil())

		ctx = host.NewContext(context.Background(), &dummy.Host{}, &dumbLogger{}, ioutil.Discard)
		ctx.Executor = &dumbExecutor{
			nextError: errors.New("invalid version"),
		}
		err = host.UpdateSoftware(ctx)
		Expect(err).To(BeNil())
	})

	It("cancels early when the operation times out", func() {
		ctx := host.NewContext(context.Background(), &dummy.Host{}, &dumbLogger{}, ioutil.Discard)
		ctx.Executor = &dumbExecutor{nextDuration: time.Second}

		start := time.Now()
		err := ctx.Run(host.Step{
			Func:    host.UpdateSoftware,
			Timeout: time.Millisecond,
		})

		Expect(time.Since(start) < time.Second).To(BeTrue(), "failed to timed out. Taken %v", time.Since(start))
		Expect(err).To(Equal(context.DeadlineExceeded))
	})
})

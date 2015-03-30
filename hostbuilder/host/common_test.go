package host_test

import (
	"errors"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dumbLogger struct{}

func (l *dumbLogger) Logf(format string, v ...interface{}) {}
func (l *dumbLogger) MetricsLabel(prefix string) string    { return "" }

type dumbExecutor struct {
	cmds         []string
	nextOutput   []byte
	nextError    error
	nextDuration time.Duration
}

func (e *dumbExecutor) CombinedOutput(cmd string) ([]byte, error) {
	e.cmds = append(e.cmds, cmd)
	time.Sleep(e.nextDuration)
	return e.nextOutput, e.nextError
}

var _ = Describe("RestartServices", func() {
	It("restarts the freighterd service", func() {
		e := &dumbExecutor{}
		err := host.RestartServices(&dumbLogger{}, e, make(chan struct{}))
		Expect(err).To(BeNil())
		Expect(e.cmds[0]).To(Equal("sudo service freighterd restart"))
	})

	It("cancels early when the operation times out", func() {
		start := time.Now()
		e := &dumbExecutor{nextDuration: time.Second}

		done := make(chan struct{})
		finish := make(chan error, 1)
		go func() {
			finish <- host.RestartServices(&dumbLogger{}, e, done)
		}()
		close(done)
		err := <-finish
		Expect(time.Since(start) < time.Second).To(BeTrue(), "failed to timed out. Taken %v", time.Since(start))
		Expect(err).To(Equal(host.ErrTimeout))
	})
})

var _ = Describe("UpdateSoftware", func() {
	It("update the freighterd service", func() {
		e := &dumbExecutor{}
		err := host.UpdateSoftware(&dumbLogger{}, e, "0.0.1", make(chan struct{}))
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
		e := &dumbExecutor{
			nextError: errors.New("invalid version"),
		}
		err := host.UpdateSoftware(&dumbLogger{}, e, "0.0.1", make(chan struct{}))
		Expect(err).ToNot(BeNil())

		e = &dumbExecutor{
			nextError: errors.New("invalid version"),
		}
		err = host.UpdateSoftware(&dumbLogger{}, e, "", make(chan struct{}))
		Expect(err).To(BeNil())
	})

	It("cancels early when the operation times out", func() {
		start := time.Now()
		e := &dumbExecutor{nextDuration: time.Second}

		done := make(chan struct{})
		finish := make(chan error, 1)
		go func() {
			finish <- host.UpdateSoftware(&dumbLogger{}, e, "0.0.1", done)
		}()
		close(done)
		err := <-finish
		Expect(time.Since(start) < time.Second).To(BeTrue(), "failed to timed out. Taken %v", time.Since(start))
		Expect(err).To(Equal(host.ErrTimeout))
	})
})

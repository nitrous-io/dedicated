package gce

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGCE(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host/gce")
}

var _ = Describe("waitForState", func() {
	It("waits until the state transition finishes", func() {
		err := waitForState("RUNNING", 1*time.Millisecond, 10*time.Millisecond, func() (string, error) {
			return "RUNNING", nil
		})
		Expect(err).To(BeNil(), "unexpected error while waiting for state")
	})

	It("returns error when the transition failed", func() {
		expected := fmt.Errorf("no idea")
		err := waitForState("RUNNING", 1*time.Millisecond, 10*time.Millisecond, func() (string, error) {
			return "FAILED", expected
		})
		Expect(err).To(Equal(expected), "unexpected error")
	})

	It("times out when the transition took too long", func() {
		var counter int
		err := waitForState("RUNNING", 2*time.Millisecond, 3*time.Millisecond, func() (string, error) {
			if counter == 2 {
				return "RUNNING", nil
			} else {
				counter++
				return "INPROGRESS", nil
			}
		})
		Expect(err).ToNot(BeNil(), "expected timeout error")
	})
})

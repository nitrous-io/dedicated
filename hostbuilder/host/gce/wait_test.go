package gce

import (
	"fmt"
	"net/http"
	"time"

	"code.google.com/p/google-api-go-client/compute/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

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

var _ = Describe("refreshOp", func() {
	It("returns the state of operation", func() {
		service, _ := compute.New(&http.Client{
			Transport: &FakeRoundTripper{
				message: getOperationResponse,
				status:  http.StatusOK,
			},
		})
		h := &Host{service: service}

		state, err := h.refreshOp("asia-east1-c", &compute.Operation{
			Name: "operation-1415261252844-5072c303d79e1-1d2317ca-32d90950",
		})()
		Expect(err).To(BeNil())
		Expect(state).To(Equal("DONE"))
	})
})

var _ = Describe("refreshInstanceState", func() {
	It("returns the state of the instance", func() {
		service, _ := compute.New(&http.Client{
			Transport: &FakeRoundTripper{
				message: getInstanceResponse,
				status:  http.StatusOK,
			},
		})
		h := &Host{service: service}

		state, err := h.refreshInstanceState("asia-east1-c", "supersonic-harrenhal-host-55")()
		Expect(err).To(BeNil())
		Expect(state).To(Equal("RUNNING"))
	})
})

package digitalocean_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/digitalocean/godo"
	"github.com/nitrous-io/hostbuilder/host/digitalocean"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("WaitForAction", func() {
	It("waits until the action is completed", func() {
		var service godo.ActionsService = testActionService{
			1: {
				responses: []*actionResponse{
					&actionResponse{&godo.Action{Status: "in-progress"}, nil},
					&actionResponse{&godo.Action{Status: "completed"}, nil},
				},
			},
		}

		a, err := digitalocean.WaitForAction(service, 1, time.Millisecond, 5*time.Millisecond)
		Expect(err).To(BeNil(), "unexpected error")
		Expect(a.Status).To(Equal("completed"))
	})

	It("returns error when we cant retrieve the action", func() {
		var service godo.ActionsService = testActionService{
			1: {
				responses: []*actionResponse{
					&actionResponse{&godo.Action{Status: "in-progress"}, nil},
					&actionResponse{nil, errors.New("invalid response")},
				},
			},
		}

		_, err := digitalocean.WaitForAction(service, 1, time.Millisecond, 5*time.Millisecond)
		Expect(err).ToNot(BeNil(), "expect error when failed to get the action")
	})

	It("returns error when the action failed to complete", func() {
		var service godo.ActionsService = testActionService{
			1: {
				responses: []*actionResponse{
					&actionResponse{&godo.Action{Status: "in-progress"}, nil},
					&actionResponse{&godo.Action{Status: "errored"}, nil},
				},
			},
		}

		_, err := digitalocean.WaitForAction(service, 1, time.Millisecond, 5*time.Millisecond)
		Expect(err).ToNot(BeNil(), "expect error when action status is errored")
	})

	It("times out when the action takes too long to complete", func() {
		var service godo.ActionsService = testActionService{
			1: {
				responses: []*actionResponse{
					&actionResponse{&godo.Action{Status: "in-progress"}, nil},
					&actionResponse{&godo.Action{Status: "in-progress"}, nil},
					&actionResponse{&godo.Action{Status: "in-progress"}, nil},
				},
			},
		}

		_, err := digitalocean.WaitForAction(service, 1, time.Millisecond, 2*time.Millisecond)
		Expect(err).ToNot(BeNil(), "expect error timed out")
	})
})

type actionResponses struct {
	counter   int
	responses []*actionResponse
}

type actionResponse struct {
	action *godo.Action
	err    error
}

type testActionService map[int]*actionResponses

func (t testActionService) List(opts *godo.ListOptions) ([]godo.Action, *godo.Response, error) {
	return nil, nil, nil
}

func (t testActionService) Get(id int) (*godo.Action, *godo.Response, error) {
	a, ok := t[id]
	if !ok {
		return nil, nil, fmt.Errorf("failed to find %d", id)
	}
	a.counter = a.counter + 1
	if a.counter > len(a.responses) {
		return nil, nil, fmt.Errorf("requested %d number of actions but only got %d", a.counter, len(a.responses))
	}
	r := a.responses[a.counter-1]
	return r.action, nil, r.err
}

package digitalocean_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/digitalocean/godo"
	"github.com/nitrous-io/dedicated/hostbuilder/host/digitalocean"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
)

var _ = Describe("WaitForAction", func() {
	var info = map[string]interface{}{}

	It("waits until the action is completed", func() {
		var service godo.ActionsService = testActionService{
			1: {
				responses: []*actionResponse{
					&actionResponse{&godo.Action{Status: "in-progress"}, nil},
					&actionResponse{&godo.Action{Status: "completed"}, nil},
				},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		a, err := digitalocean.WaitForAction(ctx, service, 1, 10*time.Millisecond, info)
		Expect(err).To(BeNil(), "unexpected error")
		Expect(a.Status).To(Equal("completed"))
	})

	It("returns error when we can't retrieve the action", func() {
		var service godo.ActionsService = testActionService{
			1: {
				responses: []*actionResponse{
					&actionResponse{&godo.Action{Status: "in-progress"}, nil},
					&actionResponse{nil, errors.New("invalid response")},
				},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_, err := digitalocean.WaitForAction(ctx, service, 1, 10*time.Millisecond, info)
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
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		_, err := digitalocean.WaitForAction(ctx, service, 1, 10*time.Millisecond, info)
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
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
		defer cancel()

		_, err := digitalocean.WaitForAction(ctx, service, 1, time.Millisecond, info)
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

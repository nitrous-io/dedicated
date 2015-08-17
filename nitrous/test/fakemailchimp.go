package test

import (
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailchimp"
)

type FakeMailChimp struct {
	SubscribeCalled  int
	SubscribeListIDs []string
	SubscribeOpts    []*mailchimp.SubscribeOptions
	SubscribeErrs    []error

	UnsubscribeCalled  int
	UnsubscribeListIDs []string
	UnsubscribeEmails  []string
	UnsubscribeErrs    []error
}

func NewFakeMailChimp() *FakeMailChimp {
	return &FakeMailChimp{
		SubscribeListIDs:   []string{},
		SubscribeOpts:      []*mailchimp.SubscribeOptions{},
		SubscribeErrs:      []error{},
		UnsubscribeListIDs: []string{},
		UnsubscribeEmails:  []string{},
		UnsubscribeErrs:    []error{},
	}
}

func (f *FakeMailChimp) Subscribe(listID string, opts *mailchimp.SubscribeOptions) error {
	f.SubscribeCalled += 1
	f.SubscribeListIDs = append(f.SubscribeListIDs, listID)
	f.SubscribeOpts = append(f.SubscribeOpts, opts)
	if len(f.SubscribeErrs) != f.SubscribeCalled {
		return nil
	}
	return f.SubscribeErrs[f.SubscribeCalled-1]
}
func (f *FakeMailChimp) Unsubscribe(listID, email string) error {
	f.UnsubscribeCalled += 1
	f.UnsubscribeListIDs = append(f.UnsubscribeListIDs, listID)
	f.UnsubscribeEmails = append(f.UnsubscribeEmails, email)
	if len(f.UnsubscribeErrs) != f.UnsubscribeCalled {
		return nil
	}
	return f.UnsubscribeErrs[f.UnsubscribeCalled-1]
}

package test

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

type FakeSubscriptionProvider struct {
	SubscribeCalled bool
	SubscribeHost   *host.Host
	SubscribePlan   *plan.Plan
	SubscribeUser   *user.User
	SubscribeError  error
	SubscribeReturn *subscription.Subscription

	CancelCalled       bool
	CancelSubscription *subscription.Subscription
	CancelError        error

	UpgradeCalled       bool
	UpgradeSubscription *subscription.Subscription
	UpgradePlan         *plan.Plan
	UpgradeUser         *user.User
	UpgradeError        error

	HandleSubscriptionUpdatedCalled bool
	HandleSubscriptionUpdatedSubID  string
	HandleSubscriptionUpdatedError  error
	HandleSubscriptionUpdatedReturn *subscription.Subscription

	HandleSubscriptionDeletedCalled bool
	HandleSubscriptionDeletedSubID  string
	HandleSubscriptionDeletedError  error
	HandleSubscriptionDeletedReturn *subscription.Subscription

	HandleSubscriptionInvoiceCreatedCalled    bool
	HandleSubscriptionInvoiceCreatedInvoiceID string
	HandleSubscriptionInvoiceCreatedError     error
	HandleSubscriptionInvoiceCreatedReturn    *invoice.Invoice

	HandleSubscriptionInvoiceFailedCalled    bool
	HandleSubscriptionInvoiceFailedInvoiceID string
	HandleSubscriptionInvoiceFailedError     error
	HandleSubscriptionInvoiceFailedReturn    *invoice.Invoice
}

func (s *FakeSubscriptionProvider) Subscribe(d *db.Database, h *host.Host, p *plan.Plan, u *user.User) (sub *subscription.Subscription, err error) {
	s.SubscribeCalled = true
	s.SubscribeHost = h
	s.SubscribePlan = p
	s.SubscribeUser = u
	return s.SubscribeReturn, s.SubscribeError
}

func (s *FakeSubscriptionProvider) Cancel(d *db.Database, sub *subscription.Subscription) error {
	s.CancelCalled = true
	s.CancelSubscription = sub
	return s.CancelError
}

func (s *FakeSubscriptionProvider) Upgrade(d *db.Database, sub *subscription.Subscription, p *plan.Plan, u *user.User) error {
	s.UpgradeCalled = true
	s.UpgradeSubscription = sub
	s.UpgradePlan = p
	s.UpgradeUser = u
	return s.UpgradeError
}

func (s *FakeSubscriptionProvider) HandleSubscriptionUpdated(d *db.Database, subID string) (*subscription.Subscription, error) {
	s.HandleSubscriptionUpdatedCalled = true
	s.HandleSubscriptionUpdatedSubID = subID
	return s.HandleSubscriptionUpdatedReturn, s.HandleSubscriptionUpdatedError
}

func (s *FakeSubscriptionProvider) HandleSubscriptionDeleted(d *db.Database, subID string) (*subscription.Subscription, error) {
	s.HandleSubscriptionDeletedCalled = true
	s.HandleSubscriptionDeletedSubID = subID
	return s.HandleSubscriptionDeletedReturn, s.HandleSubscriptionDeletedError
}

func (s *FakeSubscriptionProvider) HandleSubscriptionInvoiceCreated(d *db.Database, invID string) (*invoice.Invoice, error) {
	s.HandleSubscriptionInvoiceCreatedCalled = true
	s.HandleSubscriptionInvoiceCreatedInvoiceID = invID
	return s.HandleSubscriptionInvoiceCreatedReturn, s.HandleSubscriptionInvoiceCreatedError
}

func (s *FakeSubscriptionProvider) HandleSubscriptionInvoiceFailed(d *db.Database, invID string) (*invoice.Invoice, error) {
	s.HandleSubscriptionInvoiceFailedCalled = true
	s.HandleSubscriptionInvoiceFailedInvoiceID = invID
	return s.HandleSubscriptionInvoiceFailedReturn, s.HandleSubscriptionInvoiceFailedError
}

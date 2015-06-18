package test

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/nitrous-common/logger"
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

	HandleSubscriptionUpdatedCalled bool
	HandleSubscriptionUpdatedSubID  string
	HandleSubscriptionUpdatedError  error

	HandleSubscriptionDeletedCalled bool
	HandleSubscriptionDeletedSubID  string
	HandleSubscriptionDeletedError  error

	HandleSubscriptionInvoiceCreatedCalled    bool
	HandleSubscriptionInvoiceCreatedInvoiceID string
	HandleSubscriptionInvoiceCreatedError     error

	HandleSubscriptionInvoiceFailedCalled    bool
	HandleSubscriptionInvoiceFailedInvoiceID string
	HandleSubscriptionInvoiceFailedError     error
	HandleSubscriptionInvoiceFailedReturn    *invoice.Invoice
}

func (s *FakeSubscriptionProvider) Subscribe(d *db.Database, l *logger.Logger, h *host.Host, p *plan.Plan, u *user.User) (sub *subscription.Subscription, err error) {
	s.SubscribeCalled = true
	s.SubscribeHost = h
	s.SubscribePlan = p
	s.SubscribeUser = u
	return s.SubscribeReturn, s.SubscribeError
}

func (s *FakeSubscriptionProvider) Cancel(d *db.Database, l *logger.Logger, sub *subscription.Subscription) (err error) {
	s.CancelCalled = true
	s.CancelSubscription = sub
	return s.CancelError
}

func (s *FakeSubscriptionProvider) HandleSubscriptionUpdated(d *db.Database, l *logger.Logger, subID string) error {
	s.HandleSubscriptionUpdatedCalled = true
	s.HandleSubscriptionUpdatedSubID = subID
	return s.HandleSubscriptionUpdatedError
}

func (s *FakeSubscriptionProvider) HandleSubscriptionDeleted(d *db.Database, l *logger.Logger, subID string) error {
	s.HandleSubscriptionDeletedCalled = true
	s.HandleSubscriptionDeletedSubID = subID
	return s.HandleSubscriptionDeletedError
}

func (s *FakeSubscriptionProvider) HandleSubscriptionInvoiceCreated(d *db.Database, l *logger.Logger, invID string) error {
	s.HandleSubscriptionInvoiceCreatedCalled = true
	s.HandleSubscriptionInvoiceCreatedInvoiceID = invID
	return s.HandleSubscriptionInvoiceCreatedError
}

func (s *FakeSubscriptionProvider) HandleSubscriptionInvoiceFailed(d *db.Database, l *logger.Logger, invID string) (*invoice.Invoice, error) {
	s.HandleSubscriptionInvoiceFailedCalled = true
	s.HandleSubscriptionInvoiceFailedInvoiceID = invID
	return s.HandleSubscriptionInvoiceFailedReturn, s.HandleSubscriptionInvoiceFailedError
}

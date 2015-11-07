package test

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	stripe "github.com/stripe/stripe-go"
)

type FakeSubscriptionProvider struct {
	SubscribeCalled bool
	SubscribeHost   *host.Host
	SubscribePlan   *plan.Plan
	SubscribeUser   *user.User
	SubscribeOrg    *org.Org
	SubscribeError  error
	SubscribeReturn *subscription.Subscription

	SubscribeOrgPlanCalled bool
	SubscribeOrgPlanPlan   *plan.Plan
	SubscribeOrgPlanOrg    *org.Org
	SubscribeOrgPlanError  error
	SubscribeOrgPlanReturn *subscription.Subscription

	CancelCalled       bool
	CancelSubscription *subscription.Subscription
	CancelError        error

	ReactivateCalled       bool
	ReactivateSubscription *subscription.Subscription
	ReactivateError        error

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

	HandleChargeSucceededCalled bool
	HandleChargeSucceededEvent  *stripe.Event
	HandleChargeSucceededError  error

	HandleChargeFailedCalled bool
	HandleChargeFailedEvent  *stripe.Event
	HandleChargeFailedError  error
}

func (s *FakeSubscriptionProvider) Subscribe(d *db.Database, h *host.Host, p *plan.Plan, u *user.User, o *org.Org) (sub *subscription.Subscription, err error) {
	s.SubscribeCalled = true
	s.SubscribeHost = h
	s.SubscribePlan = p
	s.SubscribeUser = u
	s.SubscribeOrg = o
	return s.SubscribeReturn, s.SubscribeError
}

func (s *FakeSubscriptionProvider) SubscribeOrgPlan(d *db.Database, p *plan.Plan, o *org.Org) (sub *subscription.Subscription, err error) {
	s.SubscribeOrgPlanCalled = true
	s.SubscribeOrgPlanPlan = p
	s.SubscribeOrgPlanOrg = o
	return s.SubscribeOrgPlanReturn, s.SubscribeOrgPlanError
}

func (s *FakeSubscriptionProvider) Cancel(d *db.Database, sub *subscription.Subscription) error {
	s.CancelCalled = true
	s.CancelSubscription = sub
	return s.CancelError
}

func (s *FakeSubscriptionProvider) Reactivate(d *db.Database, sub *subscription.Subscription) error {
	s.ReactivateCalled = true
	s.ReactivateSubscription = sub
	return s.ReactivateError
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

func (s *FakeSubscriptionProvider) HandleChargeSucceeded(d *db.Database, e *stripe.Event) error {
	s.HandleChargeSucceededCalled = true
	s.HandleChargeSucceededEvent = e
	return s.HandleChargeSucceededError
}

func (s *FakeSubscriptionProvider) HandleChargeFailed(d *db.Database, e *stripe.Event) error {
	s.HandleChargeFailedCalled = true
	s.HandleChargeFailedEvent = e
	return s.HandleChargeFailedError
}

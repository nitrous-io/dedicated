package host

import (
	"errors"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
)

type condition struct {
	freePlan            bool
	billingProfileState string
	isOrg               bool
	hasHosts            bool
}

var (
	ErrFreePlanNotAllowed = errors.New("not allow to create free workstation")
	ErrPaidPlanNotAllowed = errors.New("not allow to create paid workstation")
	ErrUserNoCard         = errors.New("no credit card in user billing profile")
	ErrOrgNoCard          = errors.New("no credit card in org billing profile")
	ErrMaxFreeHostReached = errors.New("free host quota reached")

	_freePlan, _paidPlan      = true, false
	_noCard, _isFree, _isPaid = billingprofile.StateNoCard, billingprofile.StateFree, billingprofile.StatePaid
	_isOrg, _indiv            = true, false
	_zeroHosts, _someHosts    = false, true

	decisionTable = map[condition]error{
		condition{_freePlan, _noCard, _indiv, _zeroHosts}: ErrFreePlanNotAllowed,
		condition{_freePlan, _isFree, _indiv, _zeroHosts}: nil,
		condition{_freePlan, _isPaid, _indiv, _zeroHosts}: nil,

		condition{_freePlan, _noCard, _indiv, _someHosts}: ErrMaxFreeHostReached,
		condition{_freePlan, _isFree, _indiv, _someHosts}: ErrMaxFreeHostReached,
		condition{_freePlan, _isPaid, _indiv, _someHosts}: ErrMaxFreeHostReached,

		condition{_freePlan, _noCard, _isOrg, _zeroHosts}: ErrFreePlanNotAllowed,
		condition{_freePlan, _isFree, _isOrg, _zeroHosts}: ErrFreePlanNotAllowed,
		condition{_freePlan, _isPaid, _isOrg, _zeroHosts}: ErrFreePlanNotAllowed,

		condition{_freePlan, _noCard, _isOrg, _someHosts}: ErrFreePlanNotAllowed,
		condition{_freePlan, _isFree, _isOrg, _someHosts}: ErrFreePlanNotAllowed,
		condition{_freePlan, _isPaid, _isOrg, _someHosts}: ErrFreePlanNotAllowed,

		condition{_paidPlan, _noCard, _indiv, _zeroHosts}: ErrUserNoCard,
		condition{_paidPlan, _isFree, _indiv, _zeroHosts}: ErrPaidPlanNotAllowed,
		condition{_paidPlan, _isPaid, _indiv, _zeroHosts}: nil,

		condition{_paidPlan, _noCard, _indiv, _someHosts}: ErrUserNoCard,
		condition{_paidPlan, _isFree, _indiv, _someHosts}: ErrPaidPlanNotAllowed,
		condition{_paidPlan, _isPaid, _indiv, _someHosts}: nil,

		condition{_paidPlan, _noCard, _isOrg, _zeroHosts}: ErrOrgNoCard,
		condition{_paidPlan, _isFree, _isOrg, _zeroHosts}: ErrPaidPlanNotAllowed,
		condition{_paidPlan, _isPaid, _isOrg, _zeroHosts}: nil,

		condition{_paidPlan, _noCard, _isOrg, _someHosts}: ErrOrgNoCard,
		condition{_paidPlan, _isFree, _isOrg, _someHosts}: ErrPaidPlanNotAllowed,
		condition{_paidPlan, _isPaid, _isOrg, _someHosts}: nil,
	}
)

func CreateCheck(pl *plan.Plan, bpState string, isOrg, hasHosts bool) error {
	cond := condition{
		freePlan:            pl.IsFree(),
		billingProfileState: bpState,
		isOrg:               isOrg,
		hasHosts:            hasHosts,
	}

	if e, ok := decisionTable[cond]; ok {
		return e
	}

	// TODO We should return an error for an unhandled condition after we
	// refactor so that "delinquent" and "payment_due" billing profile states
	// are also handled here.
	logrus.Warnf("unhandled condition determining if user can create host: %s", cond)
	return nil
}

func (c *condition) String() string {
	var (
		plan, context string
	)

	if c.freePlan {
		plan = "free"
	} else {
		plan = "paid"
	}

	if c.isOrg {
		context = "org"
	} else {
		context = "individual"
	}

	return fmt.Sprintf("{plan: %s, billing profile state: %s, context: %s, has hosts: %v}",
		plan, c.billingProfileState, context, c.hasHosts)
}

#= require ./subscription

App.OrgSubscription = klass = App.Subscription.extend
  orgSlug: null
  plans: null

  canUpgrade: (->
    return false if @get('hostIsTransitioning')
    return true unless curPlan = @get('plan')
    return false unless plans = @get('plans')

    plans.any (plan) =>
      plan.get('isPaidPlan') && !plan.get('hidden') && (plan.get('price') > @get('plan.price'))
  ).property 'plans.@each', 'plan', 'plan.price', 'hostIsTransitioning'

klass.reopenClass
  url: '/orgs/:orgSlug/subscriptions'

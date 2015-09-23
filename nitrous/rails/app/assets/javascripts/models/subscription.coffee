#= require ./model
#= require ../mixins/grouped-invoice-items-mixin

attr = SM.attr

App.Subscription = klass = App.Model.extend App.GroupedInvoiceItemsMixin,
  slug: attr('string')
  planId: attr('number', as: 'plan_id')
  orgId: attr('number', as: 'org_id')
  billingProfileId: attr('number', as: 'billing_profile_id')
  state: attr('string', as: 'state')
  currentPeriodStart: attr('date', as: 'current_period_start')
  currentPeriodEnd: attr('date', as: 'current_period_end')
  createdAt: attr('date', as: 'created_at')
  updatedAt: attr('date', as: 'updated_at')
  hostSlug: attr('string', as: 'host_slug')
  cancelPending: attr('boolean', as: 'cancel_pending')

  _plan: attr('object', as: 'plan')

  plan: (->
    p = App.Plan.create()
    p.load @get('_plan')
    p
  ).property '_plan'

  host: (->
    if hostSlug = @get('hostSlug')
      App.get('hosts').findBy('slug', hostSlug)
  ).property 'hostSlug', 'App.hosts.@each'

  totalPriceInDollars: (->
    return unless @get 'plan'
    @get('plan.price') / 100.0
  ).property 'plan'

  hostIsTransitioning: (->
    !(@get('host.isRunning') || @get('host.isStopped'))
  ).property 'host.isRunning', 'host.isStopped'

  canUpgrade: (->
    return false if @get('hostIsTransitioning')
    return true unless curPlan = @get('plan')
    App.get('eligiblePlans').any (plan) =>
      plan.get('isPaidPlan') && !plan.get('hidden') && (curPlan.get('org') == plan.get('org')) && (plan.get('price') > @get('plan.price'))
  ).property 'App.eligiblePlans.@each', 'plan', 'hostIsTransitioning'

  cancel: ->
    Em.RSVP.Promise.cast Em.$.ajax
      type: 'DELETE',
      url: "/api/v0/subscriptions/#{@get 'slug'}/cancel",
      headers: { Authorization: "Bearer #{App.get 'token'}" }

  reactivate: ->
    Em.RSVP.Promise.cast Em.$.ajax
      type: 'POST',
      url: "/api/v0/subscriptions/#{@get 'slug'}/reactivate",
      headers: { Authorization: "Bearer #{App.get 'token'}" }

  upgrade: (planId)->
    Em.RSVP.Promise.cast Em.$.ajax
      type: 'POST',
      url: "/api/v0/subscriptions/#{@get 'slug'}/upgrade",
      headers: { Authorization: "Bearer #{App.get 'token'}" },
      data: { plan_id: planId }

  # Get the estimated pro-rated cost of upgrading to the given plan.
  upgradeEstimate: (planId)->
    Em.RSVP.Promise.cast Em.$.ajax
      type: 'GET',
      url: "/api/v0/subscriptions/#{@get 'slug'}/upgrade",
      headers: { Authorization: "Bearer #{App.get 'token'}" },
      data: { plan_id: planId }

klass.reopenClass
  url: '/subscriptions'
  primaryKey: 'slug'
  rootKey: 'subscription'
  collectionKey: 'subscriptions'

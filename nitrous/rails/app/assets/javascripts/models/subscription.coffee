#= require ./model
#= require ../mixins/grouped-invoice-items-mixin

attr = SM.attr

App.Subscription = klass = App.Model.extend App.GroupedInvoiceItemsMixin,
  slug: attr('string')
  planId: attr('number', as: 'plan_id')
  billingProfileId: attr('number', as: 'billing_profile_id')
  state: attr('string', as: 'state')
  currentPeriodStart: attr('date', as: 'current_period_start')
  currentPeriodEnd: attr('date', as: 'current_period_end')
  createdAt: attr('date', as: 'created_at')
  updatedAt: attr('date', as: 'updated_at')
  hostSlug: attr('string', as: 'host_slug')
  cancelPending: attr('boolean', as: 'cancel_pending')

  plan: (->
    if planId = @get('planId')
      App.get('plans').findBy('id', planId)
  ).property 'planId', 'App.plans.@each'

  totalPriceInDollars: (->
    return unless @get 'plan'
    # We grandfathered hosts that are on legacy plans to be subscription at July 2015 and made it free for first month
    # This is to show $0.0 for the first month of the subscriptions
    startDate = @get('currentPeriodStart')
    isGrandfathered = @get('plan.slug').indexOf('-legacy') >= 0 && startDate.getYear() == 115 && startDate.getMonth() == 6
    return 0.0 if isGrandfathered
    @get('plan.price') / 100.0
  ).property 'plan'

  canUpgrade: (->
    return true unless @get 'plan'
    App.get('eligiblePlans').any (plan) =>
      plan.get('isPaidPlan') && !plan.get('hidden') && (plan.get('price') > @get('plan.price'))
  ).property 'App.eligiblePlans.@each', 'plan'

  cancel: ->
    new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'DELETE',
        url: "/api/v0/subscriptions/#{@get 'slug'}/cancel",
        headers: { Authorization: "Bearer #{App.get 'token'}" },
      ).done((retData, status, jqxhr) =>
        resolve(retData)
      ).fail (jqxhr) =>
        reject(jqxhr)

  upgrade: (plan_id)->
    new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'POST',
        url: "/api/v0/subscriptions/#{@get 'slug'}/upgrade",
        headers: { Authorization: "Bearer #{App.get 'token'}" },
        data: { plan_id: plan_id }
      ).done((retData, status, jqxhr) =>
        resolve(retData)
      ).fail (jqxhr) =>
        reject(jqxhr)

  upgradeEstimate: (plan_id)->
    new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'GET',
        url: "/api/v0/subscriptions/#{@get 'slug'}/upgrade",
        headers: { Authorization: "Bearer #{App.get 'token'}" },
        data: { plan_id: plan_id }
      ).done((retData, status, jqxhr) =>
        resolve(retData)
      ).fail (jqxhr) =>
        reject(jqxhr)

klass.reopenClass
  url: '/subscriptions'
  primaryKey: 'slug'
  rootKey: 'subscription'
  collectionKey: 'subscriptions'

#= require ./model
#= require ../mixins/grouped-invoice-items-mixin

attr = SM.attr

App.Subscription = klass = App.Model.extend App.GroupedInvoiceItemsMixin,
  id: attr('number')
  planId: attr('number', as: 'plan_id')
  billingProfileId: attr('number', as: 'billing_profile_id')
  state: attr('boolean', as: 'state')
  currentPeriodStart: attr('date', as: 'current_period_start')
  currentPeriodEnd: attr('date', as: 'current_period_end')
  createdAt: attr('date', as: 'created_at')
  updatedAt: attr('date', as: 'updated_at')

  plan: (->
    if planId = @get('planId')
      App.get('plans').findBy('id', planId)
  ).property 'planId', 'App.plans.@each'

  cancel: ->
    new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'PUT',
        url: "/api/v0/subscriptions/#{@get 'id'}/cancel",
        headers: { Authorization: "Bearer #{App.get 'token'}" },
      ).done((retData, status, jqxhr) =>
        resolve(retData)
      ).fail (jqxhr) =>
        reject(jqxhr)

klass.reopenClass
  url: '/subscriptions'
  primaryKey: 'id'
  rootKey: 'subscription'
  collectionKey: 'subscriptions'

App.BillingSubscriptionsComponent = Em.Component.extend
  layoutName: 'components/billing/subscriptions'
  elementId: 'billing-subscriptions'

  subscriptions: null
  subscription: null

  terminateSlugConfirmation: null

  terminateSlugConfirmationInvalid: (->
    @get('subscription.hostSlug') != @get('terminateSlugConfirmation')
  ).property 'subscription.hostSlug', 'terminateSlugConfirmation'

  hasSingleSubscription: (->
    @get('subscriptions.length') == 1
  ).property 'subscriptions.length'

  hasMultipleSubscriptions: (->
    @get('subscriptions.length') > 1
  ).property 'subscriptions.length'

  totalSubscriptionCharge: (->
    return unless subscriptions = @get('subscriptions')
    subscriptions.reduce((val, sub)->
      val + (sub.get('totalPriceInDollars') || 0)
    , 0)
  ).property 'subscriptions.@each'

  didInsertElement: ->
    @_super.apply this, arguments
    @set 'subscription', null

  actions:
    showCancelSubscription: (subscription) ->
      @set('subscription', subscription)
      ui.showModal(@$('.cancel-subscription-modal-dialog'))

    hideCancelSubscription: ->
      ui.hideModal(@$('.cancel-subscription-modal-dialog'))
      @set('subscription', null)

    cancelSubscription: (subscription) ->
      ui.hideModal(@$('.cancel-subscription-modal-dialog'))
      return unless subscription
      subscription.cancel().then ->
        App.snackbar.post('Subscription cancelled successfully.')
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, 'Failed to cancel subscription.')

App.BillingController = Em.ObjectController.extend
  subscription: null
  totalSubscriptionCharge: (->
    return unless App.get('subscriptions.content.length')
    sum = App.get('subscriptions.content').reduce (val, sub)->
      val + (sub.get('totalPriceInDollars') || 0)
    , 0
    sum
  ).property 'App.subscriptions.@each'

  actions:
    changeCard: ->
      @transitionTo 'add-card'

    payInvoice: (invoice) ->
      invoice.pay().then =>
        invoice.set('awaitingUpdate', true)
        App.snackbar.post("Payment submitted successfully.")
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to submit payment.')

    showCancelSubscription: (subscription) ->
      @set('subscription', subscription)
      ui.showModal('#cancel-subscription-modal-dialog')

    hideCancelSubscription: ->
      ui.hideModal('#cancel-subscription-modal-dialog')

    cancelSubscription: (subscription) ->
      ui.hideModal('#cancel-subscription-modal-dialog')
      return unless subscription
      subscription.cancel().then ->
        App.snackbar.post('Subscription cancelled successfully.')
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, 'Failed to cancel subscription.')

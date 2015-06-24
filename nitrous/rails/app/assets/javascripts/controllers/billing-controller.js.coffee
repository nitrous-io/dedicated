#= require jquery.payment

App.BillingController = Em.ObjectController.extend
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
      subscription.cancel().then ->
        App.snackbar.post('Cancelled subscription successfully.')
        App.refreshSubscriptions()
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, 'Failed to cancel subscription.')

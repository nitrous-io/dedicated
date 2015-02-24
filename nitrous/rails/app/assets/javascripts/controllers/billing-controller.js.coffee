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

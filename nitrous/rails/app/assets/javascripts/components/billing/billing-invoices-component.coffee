App.BillingInvoicesComponent = Em.Component.extend
  layoutName: 'components/billing/invoices'
  elementId: 'billing-invoices'

  invoices: null
  invoice: null

  actions:
    closeInvoice: ->
      ui.hideModal('#showinvoice-modal-dialog')
      @set 'invoice', null

    showInvoice: (invoice) ->
      @set 'invoice', invoice
      ui.showModal('#showinvoice-modal-dialog')

    payInvoice: (invoice) ->
      invoice.pay().then =>
        invoice.set('awaitingUpdate', true)
        App.snackbar.post("Payment submitted successfully.")
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to submit payment.')

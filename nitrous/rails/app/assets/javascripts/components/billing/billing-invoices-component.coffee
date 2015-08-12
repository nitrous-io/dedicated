App.BillingInvoicesComponent = Em.Component.extend
  layoutName: 'components/billing/invoices'
  elementId: 'billing-invoices'

  invoices: null

  actions:
    payInvoice: (invoice) ->
      invoice.pay().then =>
        invoice.set('awaitingUpdate', true)
        App.snackbar.post("Payment submitted successfully.")
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to submit payment.')

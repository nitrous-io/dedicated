#= require ./authed-route

App.BillingRoute = App.AuthedRoute.extend
  model: (params) ->
    invoices = App.Invoice.fetch().value
    App.set('invoices', invoices)
    invoices

  beforeModel: ->
    @_super.apply this, arguments
    # TODO: FFLAG payments
    if App.get('user.billingEnabled') && !App.get('billingProfile.paid')
      @transitionTo 'add-card'
    App.BillingUsage.fetchCurrentUsage()

  activate: ->
    App.set 'title', 'Billing'

  renderTemplate: ->
    @render 'billing', outlet: 'content'

#= require ./authed-route

App.BillingRoute = App.AuthedRoute.extend
  model: (params) ->
    invoices = App.Invoice.fetch().value
    App.set('invoices', invoices)
    invoices

  beforeModel: ->
    @_super.apply this, arguments
    # TODO: FFLAG payments
    unless App.get('billingProfile.paid')
      @transitionTo 'add-card'
    App.refreshUsage()

  activate: ->
    App.set 'title', 'Billing'
    Mp.track Mp.EventDashboardViewedBilling

  renderTemplate: ->
    @render 'billing', outlet: 'content'

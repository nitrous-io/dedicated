#= require ./authed-route

App.BillingRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments

    if App.get('billingProfile.isNoCardState') || App.get('billingProfile.free')
      @transitionTo 'add-card'

    App.refreshUsage()
    App.refreshInvoices()

  activate: ->
    App.set 'title', 'Billing'
    Mp.track Mp.EventDashboardViewedBilling

  renderTemplate: ->
    @render 'billing', outlet: 'content'

App.BillingRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    # TODO: FFLAG payments
    if !App.get('user.billingEnabled')
      @transitionTo '/'
    if !App.get('billingProfile.paid')
      @transitionTo 'add-card'

  activate: ->
    App.set 'title', 'Billing'

  renderTemplate: ->
    @render 'billing', outlet: 'content'

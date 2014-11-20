App.BillingRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    if !App.get('billingProfile.paid')
      @transitionTo 'addCard'

  activate: ->
    App.set 'title', 'Billing'

  renderTemplate: ->
    @render 'billing', outlet: 'content'

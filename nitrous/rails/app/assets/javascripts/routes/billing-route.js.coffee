App.BillingRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    if !App.get('user.admin')
      @transitionTo '/'
    if !App.get('billingProfile.paid')
      @transitionTo 'add-card'

  activate: ->
    App.set 'title', 'Billing'

  renderTemplate: ->
    @render 'billing', outlet: 'content'

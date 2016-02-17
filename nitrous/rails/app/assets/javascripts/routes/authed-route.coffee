App.AuthedRoute = Em.Route.extend
  beforeModel: (transition) ->
    @_super.apply this, arguments

    if !App.get('token')
      @transitionTo 'login'
      return

    user = App.get('user')

    if !user
      App.User.fetch().then (user) =>
        App.trigger 'fetchUser', user
        safeRoute = transition.targetName.indexOf("billing.") != -1 || transition.targetName.indexOf("add-card") != -1
        unless safeRoute
          @checkForBillingIssues()
      , (jqxhr) =>
        transition.abort()
        # retry in 5 seconds
        Em.run.later(this, ->
          transition.retry()
        , 5000)

  checkForBillingIssues: ->
    if App.get('user._billingProfile.has_billing_issues')
      @transitionTo 'billing'

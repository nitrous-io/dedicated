App.LogoutRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    App.clear()
    @transitionTo 'login'

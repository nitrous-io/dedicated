App.LogoutRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    App.set 'user', null
    App.set 'token', null
    @transitionTo 'login'

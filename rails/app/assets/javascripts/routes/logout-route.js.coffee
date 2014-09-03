App.LogoutRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    App.set 'token', null
    Em.run.next ->
      location.href = "/"

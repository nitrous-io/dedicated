App.HostsIndexRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    @transitionTo 'home.index'

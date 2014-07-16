App.HostsIndexRoute = Em.Route.extend
  beforeModel: ->
    @transitionTo 'home.index'

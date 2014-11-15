#= require ./authed-route

App.ContainersIndexRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    @transitionTo 'home.index'

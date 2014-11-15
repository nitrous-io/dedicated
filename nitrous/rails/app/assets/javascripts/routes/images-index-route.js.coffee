#= require ./authed-route

App.ImagesIndexRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arguments
    @transitionTo 'home.index'

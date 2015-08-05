#= require ./authed-route

App.OrgIndexRoute = App.AuthedRoute.extend
  controllerName: 'org'

  beforeModel: ->
    @transitionTo 'org.members'

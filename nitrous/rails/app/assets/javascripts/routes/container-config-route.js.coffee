#= require ./authed-route

App.ContainerConfigRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/config', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    App.set 'title', model.get('displayName') + ' › Config'
    controller.loadConfig()
    Mp.track Mp.EventDashboardViewedContainerConfig

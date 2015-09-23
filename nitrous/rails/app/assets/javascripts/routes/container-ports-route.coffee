#= require ./authed-route

App.ContainerPortsRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/ports', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    App.set 'title', model.get('displayName') + ' › Ports'
    controller.loadConfig()
    Mp.track Mp.EventDashboardViewedContainerPorts

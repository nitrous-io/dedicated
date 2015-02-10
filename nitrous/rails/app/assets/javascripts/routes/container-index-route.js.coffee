#= require ./authed-route

App.ContainerIndexRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/index', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName')
    controller.clear()
    Mp.track Mp.EventDashboardViewedContainer

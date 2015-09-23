#= require ./authed-route

App.ContainerCustomDomainRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/custom-domain', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    App.set 'title', model.get('displayName') + ' › Custom Domain'
    Mp.track Mp.EventDashboardViewedContainerCustomDomain

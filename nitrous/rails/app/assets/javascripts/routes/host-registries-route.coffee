#= require ./authed-route

App.HostRegistriesRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/registries', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    if (registries = model.get 'registries') && registries.get('loaded')
      registries.reload()
    App.set 'title', model.get('displayName') + ' › Registries'
    Mp.track Mp.EventDashboardViewedHostRegistries

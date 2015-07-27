#= require ./authed-route

App.HostRegistriesNewRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/registries-new', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    App.set 'title', model.get('displayName') + ' › Connect to Registry'

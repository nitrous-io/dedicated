#= require ./authed-route

App.AppPortsRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'app/ports', into: 'app'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    App.set 'title', model.get('displayName') + ' › Ports'

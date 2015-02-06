#= require ./authed-route

App.ContainerConsoleRoute = App.AuthedRoute.extend
  deactivate: ->
    @get('controller.socket')?.disconnect()
    @set 'controller.model', null

  renderTemplate: ->
    @render 'container/console', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' › Console'

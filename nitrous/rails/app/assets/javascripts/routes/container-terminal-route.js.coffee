#= require ./authed-route

App.ContainerTerminalRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Terminal'

  deactivate: ->
    @get('controller.socket')?.disconnect()
    @set 'controller.model', null

  renderTemplate: ->
    @render 'container/terminal', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' › Terminal'

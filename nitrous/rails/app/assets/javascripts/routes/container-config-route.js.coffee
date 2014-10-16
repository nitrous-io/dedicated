App.ContainerConfigRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/config', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    controller.loadConfig()

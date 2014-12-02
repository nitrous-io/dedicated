#= require ./ide-route

App.BoxTerminalRoute = App.IdeRoute.extend
  setupController: (controller, model) ->
    @_super.apply this, arguments
    boxIdeController = @controllerFor('boxIde')
    boxIdeController.set 'content', model
    boxIdeController.initialize()

  renderTemplate: ->
    @render 'boxTerminal', controller: 'boxIde'

  deactivate: ->
    @_super()
    controller = @controllerFor 'boxIde'
    controller.reset()

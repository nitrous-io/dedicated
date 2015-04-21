App.FreeTierRoute = Em.Route.extend
  setupController: (controller) ->
    @_super.apply this, arguments
    controller.run()
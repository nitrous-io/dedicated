App.BoxProvisioningRoute = Em.Route.extend
  deactivate: ->
    @controller.set 'content', null

App.PublicKeysNewRoute = Em.Route.extend
  model: ->
    App.PublicKey.create()

  deactivate: ->
    @controller.set 'content', null

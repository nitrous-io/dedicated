App.PublicKeysIndexRoute = Em.Route.extend
  model: ->
    App.get 'publicKeys'

  activate: ->
    App.get('publicKeys').reload()

  deactivate: ->
    @controller.set 'content', null

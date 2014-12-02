App.AccountRoute = Em.Route.extend
  model: ->
    App.get 'currentUser'

  deactivate: ->
    @controller.set 'content', null

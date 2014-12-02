App.IntroAppsRoute = Em.Route.extend
  model: ->
    App.get 'boxes.lastObject'

  deactivate: ->
    @controller.set 'content', null

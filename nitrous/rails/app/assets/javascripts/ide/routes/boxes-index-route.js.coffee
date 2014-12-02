App.BoxesIndexRoute = Em.Route.extend
  beforeModel: ->
    unless App.get('currentUser.intro_seen') || App.get('currentUser.hasUsedN2O')
      @transitionTo 'intro.index'
      return
    App.util.saveReturnTo('boxes.index')

  model: ->
    App.get 'boxes'

  activate: ->
    App.get('boxes').reload()

  deactivate: ->
    @controller.set 'content', null

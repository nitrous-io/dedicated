App.ImagesIndexRoute = Em.Route.extend
  beforeModel: ->
    @transitionTo 'home.index'

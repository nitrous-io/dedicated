App.ImagesIndexRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super.apply this, arugments
    @transitionTo 'home.index'

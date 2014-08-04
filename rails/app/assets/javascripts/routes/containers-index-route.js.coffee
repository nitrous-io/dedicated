App.ContainersIndexRoute = App.AuthedRoute.extend
  beforeModel: ->
    @_super()
    @transitionTo 'home.index'

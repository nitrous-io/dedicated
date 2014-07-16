App.ContainersIndexRoute = Em.Route.extend
  beforeModel: ->
    @transitionTo 'home.index'

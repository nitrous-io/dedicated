App.IndexRoute = Em.Route.extend
  redirect: ->
    @transitionTo 'boxes.index'

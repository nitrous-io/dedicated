App.N2oIndexRoute = Em.Route.extend
  redirect: ->
    @transitionTo 'n2o.bonus'

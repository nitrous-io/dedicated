App.IntroIndexRoute = Em.Route.extend
  redirect: ->
    @transitionTo 'intro.create_box'

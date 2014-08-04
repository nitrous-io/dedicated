App.AuthedRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get('token')
      unless App.get 'user'
        new Em.RSVP.Promise (resolve) =>
          user = App.User.fetch()
          user.one 'didLoad', ->
            App.set 'user', user
            resolve()
          user.one 'didFailToLoad', =>
            resolve()
    else
      @transitionTo 'login'

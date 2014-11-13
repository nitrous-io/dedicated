App.AuthedRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get 'token'
      unless App.get 'user'
        Ember.RSVP.Promise.all([
          App.User.fetch(),
          App.UserProfile.fetch()
        ]).then (userAndProfile) ->
          App.trigger 'fetchUser', userAndProfile[0], userAndProfile[1]
    else
      @transitionTo 'login'

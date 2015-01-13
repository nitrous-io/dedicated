App.AuthedRoute = Em.Route.extend
  beforeModel: (transition) ->
    @_super.apply this, arguments
    if App.get('token')
      if !App.get('user')
        App.User.fetch().then (user) ->
          App.trigger 'fetchUser', user
        # FIXME ADMINONLY failure handler below
        , (jqxhr) =>
          if jqxhr?.responseJSON?.error == 'beta_closed'
            App.set 'token', null
            @transitionTo 'login'
          else
            transition.abort()
            # retry in 5 seconds
            Em.run.later(this, ->
              transition.retry()
            , 5000)
    else
      @transitionTo 'login'

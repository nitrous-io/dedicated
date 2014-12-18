App.AuthedRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get 'token'
      unless App.get 'user'
        App.User.fetch().then (user) ->
          App.trigger 'fetchUser', user
        # FIXME ADMINONLY failure handler below
        , (jqxhr) =>
          if jqxhr?.responseJSON?.error == 'beta_closed'
            if window.confirm("#{jqxhr.responseJSON.error_description}\n\nGo to Nitrous.IO?")
              window.location.href = "https://www.nitrous.io"
            else
              @transitionTo 'login'
          else
            @transitionTo 'login'
    else
      @transitionTo 'login'

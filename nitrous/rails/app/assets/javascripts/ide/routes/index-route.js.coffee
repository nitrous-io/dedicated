App.IndexRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get 'token'
      unless App.get 'user'
        App.CurrentUser.fetch().then (user) ->
          App.trigger 'fetchUser', user
    else
      # redirect to login screen if user hasn't logged in
      window.location = window.location.href.replace('/ide', '/app#login')

  afterModel: ->
    @transitionTo 'ide'

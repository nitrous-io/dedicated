App.IndexRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get 'token'
      unless App.get 'user'
        App.CurrentUser.fetch().then (user) ->
          App.trigger 'fetchUser', user
    else
      @transitionTo 'login'

  redirect: ->
    @transitionTo 'ide'

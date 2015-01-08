App.AdminRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get('user.admin')
      return

    unless App.get 'token'
      @transitionTo 'login'

    unless App.get('user')
      App.User.fetch().then (user) ->
        App.trigger 'fetchUser', user
        unless user.get('admin')
          @transitionTo 'home'

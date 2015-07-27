App.ResetPasswordRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Reset Password'
    App.set 'user', null
    App.clearToken()

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'reset-password', outlet: 'topnav', controller: 'topnav'

  setupController: (controller) ->
    controller.setProperties
      password: null
      confirmPassword: null

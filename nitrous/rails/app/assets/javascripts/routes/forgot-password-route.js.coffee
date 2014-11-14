App.ForgotPasswordRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Forgot Password'
    App.set 'user', null
    App.set 'token', null

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'forgot-password', outlet: 'topnav', controller: 'topnav'

  setupController: (controller) ->
    controller.setProperties
      login: null

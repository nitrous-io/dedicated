App.LoginRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get('token')
      @transitionTo 'home'

  activate: ->
    App.set 'title', 'Log In'
    App.set 'user', null
    App.set 'token', null

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'login', outlet: 'topnav', controller: 'topnav'

  setupController: (controller) ->
    controller.setProperties
      confirmationLapsed: false
      login: null
      password: null
      error: null
      loggingIn: false
    App.on 'confirmation-lapsed', ->
      controller.setProperties
        confirmationLapsed: true
        loggingIn: false
    mp.track 'Login Page Viewed'

  deactivate: ->
    controller = @get 'controller'
    App.off 'confirmation-lapsed'

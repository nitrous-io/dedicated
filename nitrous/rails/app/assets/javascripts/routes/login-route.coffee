App.LoginRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get('token')
      @transitionTo 'home'

  activate: ->
    App.set 'title', 'Log In'
    App.set 'user', null
    App.clearToken()

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'login', outlet: 'topnav', controller: 'topnav'

  setupController: (controller) ->
    controller.setProperties
      login: null
      password: null
      error: null
      loggingIn: false
    App.on 'confirmation-lapsed', ->
      controller.setProperties
        loggingIn: false
    Mp.track Mp.EventViewedLogin

  deactivate: ->
    controller = @get 'controller'
    App.off 'confirmation-lapsed'
    App.set 'confirmationLapsed', false

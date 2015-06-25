App.SignupRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get('token')
      @transitionTo 'home'

  activate: ->
    App.set 'title', 'New Account'
    App.set 'user', null
    App.clearToken()

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'signup', outlet: 'topnav', controller: 'topnav'

  setupController: (controller) ->
    controller.clear()
    controller.handleIncomingEmail()
    mpProps = if email = controller.get('incomingEmail')
      mpProps = { email: email }
    else
      {}
    Mp.track Mp.EventViewedSignUp, mpProps

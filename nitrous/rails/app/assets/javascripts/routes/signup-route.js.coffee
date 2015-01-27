App.SignupRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get('token')
      @transitionTo 'home'

  activate: ->
    App.set 'title', 'New Account'
    App.set 'user', null
    App.set 'token', null

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'signup', outlet: 'topnav', controller: 'topnav'

  setupController: (controller) ->
    controller.clear()
    controller.handleIncomingEmail()
    if mixpanel?
      data = {}
      if (email = controller.get('incomingEmail'))
        mixpanel.people.set {$email: email}
        data = {email: email}
      mixpanel.track "Signup Page Viewed", data

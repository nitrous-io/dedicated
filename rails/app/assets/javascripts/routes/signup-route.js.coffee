App.SignupRoute = Em.Route.extend
  beforeModel: ->
    @_super.apply this, arguments
    if App.get('token')
      @transitionTo 'home'

  activate: ->
    App.set 'title', 'Sign Up'
    App.set 'user', null
    App.set 'token', null

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'signup', outlet: 'topnav', controller: 'topnav'

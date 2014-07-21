App.SignupRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Sign Up'
    App.set 'currentUser', null

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'signup', outlet: 'topnav', controller: 'topnav'

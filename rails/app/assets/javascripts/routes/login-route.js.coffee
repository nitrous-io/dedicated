App.LoginRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Log In'
    App.set 'currentUser', null

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'login', outlet: 'topnav', controller: 'topnav'

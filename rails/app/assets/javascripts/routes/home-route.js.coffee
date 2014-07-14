App.HomeRoute = Em.Route.extend
  activate: ->
    App.set 'title', null
    App.set 'currentUser', 'petejkim'

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'home', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'home', outlet: 'sidebar', controller: 'sidebar'

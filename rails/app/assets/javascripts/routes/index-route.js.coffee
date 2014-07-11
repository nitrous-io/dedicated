App.IndexRoute = Em.Route.extend
  activate: ->
    App.set 'title', null
    App.set 'currentUser', 'petejkim'

  renderTemplate: ->
    @_super()
    @render 'topnav',  into: 'index', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'index', outlet: 'sidebar', controller: 'sidebar'
    @render 'content', into: 'index', outlet: 'content', controller: 'index'

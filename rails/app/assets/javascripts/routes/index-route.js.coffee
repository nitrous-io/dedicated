App.IndexRoute = Em.Route.extend
  renderTemplate: ->
    @_super()
    @render 'topnav',  into: 'index', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'index', outlet: 'sidebar', controller: 'sidebar'

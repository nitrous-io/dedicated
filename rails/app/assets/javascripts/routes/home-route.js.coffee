App.HomeRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', null
    App.set 'hosts', App.Host.fetch().value
    App.set 'containers', App.Container.fetch().value
    App.set 'regions', App.Region.fetch().value
    App.set 'plans', App.Plan.fetch().value

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'home', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'home', outlet: 'sidebar', controller: 'sidebar'

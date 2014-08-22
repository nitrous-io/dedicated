App.HomeRoute = App.AuthedRoute.extend
  beforeModel: ->
    App.set 'hosts', App.Host.fetch().value
    App.set 'containers', App.Container.fetch().value
    App.set 'regions', App.Region.fetch().value
    App.set 'plans', App.Plan.fetch().value

  activate: ->
    App.set 'title', null

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'home', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'home', outlet: 'sidebar', controller: 'sidebar'

App.HomeRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', null

  renderTemplate: ->
    @_super()
    @render 'topnav', into: 'home', outlet: 'topnav',  controller: 'topnav'
    @render 'sidebar', into: 'home', outlet: 'sidebar', controller: 'sidebar'

  setupController: (controller) ->
    App.Host.fetch().then (records) =>
      @controllerFor('sidebar').set('model', records)

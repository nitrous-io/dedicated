App.HostIndexRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/index', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName')
    model.fetchStats()

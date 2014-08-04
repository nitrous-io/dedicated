App.HostIndexRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Project X'

  renderTemplate: ->
    @render 'host/index', into: 'host'

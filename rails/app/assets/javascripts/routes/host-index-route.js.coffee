App.HostIndexRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Project X'

  renderTemplate: ->
    @render 'host/index', into: 'host'

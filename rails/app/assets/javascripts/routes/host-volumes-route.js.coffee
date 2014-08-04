App.HostVolumesRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Project X > Volumes'

  renderTemplate: ->
    @render 'host/volumes', into: 'host'

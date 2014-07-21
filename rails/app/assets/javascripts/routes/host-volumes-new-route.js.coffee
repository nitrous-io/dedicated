App.HostVolumesNewRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'New Volume'

  renderTemplate: ->
    @render 'host/volumes-new', into: 'host'

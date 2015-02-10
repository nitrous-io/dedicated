#= require ./authed-route

App.HostVolumesRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/volumes', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    if (volumes = model.get 'volumes') && volumes.get('loaded')
      volumes.reload()
    App.set 'title', model.get('displayName') + ' › Volumes'
    Mp.track Mp.EventDashboardViewedHostVolumes

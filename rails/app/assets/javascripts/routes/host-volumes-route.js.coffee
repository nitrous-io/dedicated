App.HostVolumesRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/volumes', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    volumes = model.get('volumes')
    volumes.reload() if volumes.get('loaded')
    App.set 'title', model.get('displayName') + ' » Volumes'

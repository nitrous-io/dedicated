#= require ./authed-route

App.HostImagesRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/images', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    if (images = model.get 'images') && images.get('loaded')
      images.reload()
    App.set 'title', model.get('displayName') + ' › Images'
    Mp.track Mp.EventDashboardViewedHostImages

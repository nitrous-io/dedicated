#= require ./authed-route

App.HostVolumesNewRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/volumes-new', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    App.set 'title', model.get('displayName') + ' › New Volume'

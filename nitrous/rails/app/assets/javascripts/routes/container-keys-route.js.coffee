#= require ./authed-route

App.ContainerKeysRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/keys', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    if (keys = model.get 'keys') && keys.get('loaded')
      keys.reload()
    App.set 'title', model.get('displayName') + ' › Keys'

#= require ./authed-route

App.ContainerKeysRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/keys', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    keys = model.get 'keys'
    if keys && keys.get('loaded')
      keys.reload()
    App.set 'title', model.get('displayName') + ' › SSH Keys'

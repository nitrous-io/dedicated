#= require ./authed-route

App.AppKeysRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'app/keys', into: 'app'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    keys = model.get 'appContainer.keys'
    if keys && keys.get('loaded')
      keys.reload()
    App.set 'title', model.get('displayName') + ' › SSH Keys'

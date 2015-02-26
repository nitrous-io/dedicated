#= require ./authed-route

App.ContainerKeysRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/keys', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    userKeys = model.get 'userKeys'
    if userKeys && userKeys.get('loaded')
      userKeys.reload()
    model.loadContainerKeys()
    App.set 'title', model.get('displayName') + ' › SSH Keys'
    Mp.track Mp.EventDashboardViewedContainerKeys

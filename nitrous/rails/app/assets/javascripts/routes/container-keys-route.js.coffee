#= require ./authed-route

App.ContainerKeysRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/keys', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()

    userKeys = model.get 'userKeys'
    if userKeys
      userKeys.reload()

    containerKeys = model.get 'containerKeys'
    if containerKeys
      containerKeys.reload().catch (jqxhr) =>
        resp = jqxhr.responseJSON
        controller.set('error', true)
        if resp?.error == "container_not_running"
          controller.set 'notRunningError', true
        else
          App.postError(resp, "Failed to load container keys")

    App.set 'title', model.get('displayName') + ' › SSH Keys'
    Mp.track Mp.EventDashboardViewedContainerKeys

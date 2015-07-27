#= require ./authed-route

App.ContainerKeysNewRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/keys-new', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    App.set 'title', model.get('displayName') + ' › Register SSH Public Key'

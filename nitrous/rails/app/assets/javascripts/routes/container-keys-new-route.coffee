#= require ./authed-route

App.ContainerKeysNewRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/keys-new', into: 'container'

  afterModel: (container, transition) ->
   @_super.apply this, arguments

   if !container.get('paidFeaturesEnabled')
     transition.abort()
     App.snackbar.post("You do not have permission to add new user public keys")
     @transitionTo 'container.index', container.get 'slug'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()
    App.set 'title', model.get('displayName') + ' › Register SSH Public Key'

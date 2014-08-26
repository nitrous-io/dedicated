App.HostRegistriesRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/registries', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' » Registries'

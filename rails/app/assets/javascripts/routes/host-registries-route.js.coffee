App.HostRegistriesRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'host/registries', into: 'host'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    registries = model.get('registries')
    registries.reload() if registries.get('loaded')
    App.set 'title', model.get('displayName') + ' » Registries'

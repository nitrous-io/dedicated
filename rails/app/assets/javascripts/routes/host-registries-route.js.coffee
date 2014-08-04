App.HostRegistriesRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Project X > Registries'

  renderTemplate: ->
    @render 'host/registries', into: 'host'

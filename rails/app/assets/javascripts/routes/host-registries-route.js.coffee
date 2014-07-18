App.HostRegistriesRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Project X > Registries'

  renderTemplate: ->
    @render 'host/registries', into: 'host'

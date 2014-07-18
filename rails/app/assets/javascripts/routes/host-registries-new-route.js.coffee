App.HostRegistriesNewRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Add Registry'

  renderTemplate: ->
    @render 'host/registries-new', into: 'host'

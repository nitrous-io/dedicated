App.HostRegistriesNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Add Registry'

  renderTemplate: ->
    @render 'host/registries-new', into: 'host'

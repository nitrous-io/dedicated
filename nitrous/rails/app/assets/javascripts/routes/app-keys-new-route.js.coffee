#= require ./container-keys-new-route

App.AppKeysNewRoute = App.ContainerKeysNewRoute.extend
  renderTemplate: ->
    @render 'app/keys-new', into: 'app'

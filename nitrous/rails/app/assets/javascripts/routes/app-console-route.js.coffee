#= require ./container-console-route

App.AppConsoleRoute = App.ContainerConsoleRoute.extend
  renderTemplate: ->
    @render 'app/console', into: 'app'

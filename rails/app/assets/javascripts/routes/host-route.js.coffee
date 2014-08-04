App.HostRoute = App.AuthedRoute.extend
  model: ->

  renderTemplate: ->
    @render 'host', outlet: 'content'

App.HostRoute = Em.Route.extend
  model: ->

  renderTemplate: ->
    @render 'host', outlet: 'content'

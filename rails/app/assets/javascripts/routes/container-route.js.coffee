App.ContainerRoute = Em.Route.extend
  model: ->

  renderTemplate: ->
    @render 'container', outlet: 'content'

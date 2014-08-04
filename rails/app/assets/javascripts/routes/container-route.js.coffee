App.ContainerRoute = App.AuthedRoute.extend
  model: ->

  renderTemplate: ->
    @render 'container', outlet: 'content'

App.HomeIndexRoute = Em.Route.extend
  renderTemplate: ->
    @render 'home/index', outlet: 'content'

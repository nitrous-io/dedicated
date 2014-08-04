App.HomeIndexRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'home/index', outlet: 'content'

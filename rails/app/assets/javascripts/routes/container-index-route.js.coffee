App.ContainerIndexRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Web App'

  renderTemplate: ->
    @render 'container/index', into: 'container'

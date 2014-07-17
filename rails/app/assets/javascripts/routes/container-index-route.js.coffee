App.ContainerIndexRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Web App'

  renderTemplate: ->
    @render 'container/index', into: 'container'

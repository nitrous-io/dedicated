App.ContainerIndexRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'container'

  renderTemplate: ->
    @render 'container/index', into: 'container'

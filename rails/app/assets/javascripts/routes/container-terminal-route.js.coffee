App.ContainerTerminalRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Terminal'

  renderTemplate: ->
    @render 'container/terminal', into: 'container'

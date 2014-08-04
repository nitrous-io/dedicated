App.ContainerTerminalRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Terminal'

  renderTemplate: ->
    @render 'container/terminal', into: 'container'

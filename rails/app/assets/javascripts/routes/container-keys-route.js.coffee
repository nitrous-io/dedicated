App.ContainerKeysRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'SSH Public Keys'

  renderTemplate: ->
    @render 'container/keys', into: 'container'

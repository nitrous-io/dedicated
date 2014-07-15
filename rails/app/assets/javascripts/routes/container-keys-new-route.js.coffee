App.ContainerKeysNewRoute = Em.Route.extend
  activate: ->
    App.set 'title', 'Add Key'

  renderTemplate: ->
    @render 'container/keys-new', into: 'container'

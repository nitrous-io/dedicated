App.ContainerKeysNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Add Key'

  renderTemplate: ->
    @render 'container/keys-new', into: 'container'

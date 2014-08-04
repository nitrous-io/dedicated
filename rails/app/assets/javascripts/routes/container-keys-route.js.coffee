App.ContainerKeysRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Web App > Keys'

  renderTemplate: ->
    @render 'container/keys', into: 'container'

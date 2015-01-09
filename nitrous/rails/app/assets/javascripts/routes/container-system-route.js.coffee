#= require ./authed-route

App.ContainerSystemRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/system', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' › System'

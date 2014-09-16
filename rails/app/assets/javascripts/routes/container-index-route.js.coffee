App.ContainerIndexRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/index', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' › General'
    if controller.get 'newContainer'
      controller.send 'start'

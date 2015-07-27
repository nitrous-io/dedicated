#= require ./authed-route

App.ContainerCollaboratorsRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'container/collaborators', into: 'container'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.clear()

    collaborators = model.get 'collaborators'
    if collaborators
      collaborators.reload()

    App.set 'title', model.get('displayName') + ' › Collaborators'

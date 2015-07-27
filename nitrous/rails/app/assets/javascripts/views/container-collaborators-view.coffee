App.ContainerCollaboratorsView = Em.View.extend
  elementId: 'container-collaborators-view'

  actions:
    toggle: (collaborator) ->
      val = collaborator.get 'pendingRemove'
      collaborator.set 'pendingRemove', !val
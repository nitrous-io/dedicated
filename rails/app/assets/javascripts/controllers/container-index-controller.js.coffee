App.ContainerIndexController = Em.ObjectController.extend
  actions:
    openTerminal: ->
      @transitionToRoute 'container.terminal'

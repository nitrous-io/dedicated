App.ContainerIndexController = Em.Controller.extend
  actions:
    openTerminal: ->
      @transitionToRoute 'container.terminal'

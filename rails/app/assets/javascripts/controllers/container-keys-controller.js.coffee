App.ContainerKeysController = Em.Controller.extend
  actions:
    addKey: ->
      @transitionToRoute 'container.keys-new'

App.ContainerKeysController = Em.Controller.extend
  actions:
    add: ->
      @transitionToRoute 'container.keys-new'

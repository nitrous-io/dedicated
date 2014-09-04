App.ContainerKeysController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.keys'

  actions:
    add: ->
      @transitionToRoute 'container.keys-new'

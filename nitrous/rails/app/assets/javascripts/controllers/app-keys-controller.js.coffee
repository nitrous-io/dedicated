App.AppKeysController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.appContainer.keys'

  actions:
    add: ->
      @transitionToRoute 'app.keys-new'

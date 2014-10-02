App.HostRegistriesController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.registries'

  actions:
    add: ->
      @transitionToRoute 'host.registries-new'

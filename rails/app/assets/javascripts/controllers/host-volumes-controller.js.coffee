App.HostVolumesController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.volumes'

  actions:
    add: ->
      @transitionToRoute 'host.volumes-new'

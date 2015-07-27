App.HostVolumesController = Lib.RecordListController.extend
  recordsBinding: Em.Binding.oneWay 'model.volumes'

  actions:
    calculateSize: (volume) ->
      volume.fetchInfo()

    add: ->
      @transitionToRoute 'host.volumes-new'

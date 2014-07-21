App.HostVolumesController = Em.Controller.extend
  actions:
    new: ->
      @transitionToRoute 'host.volumes-new'

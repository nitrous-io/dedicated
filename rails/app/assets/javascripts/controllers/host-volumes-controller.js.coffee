App.HostVolumesController = Em.ObjectController.extend
  removing: false

  clear: ->
    @setProperties
      removing: false

  actions:
    new: ->
      @transitionToRoute 'host.volumes-new'

    remove: (volumes) ->
      @set 'removing', true
      host = @get 'model'

      fns = volumes.map (volume) ->
        (cb) ->
          volume.ajax('DELETE').then ->
            cb(null)
          , (jqxhr) ->
            if errMsg = jqxhr.responseJSON?.error_description
              App.snackbar.post(errMsg.capitalize())
            cb(jqxhr)

      async.series fns, (err) =>
        host.get('volumes').reload()
        @set 'removing', false

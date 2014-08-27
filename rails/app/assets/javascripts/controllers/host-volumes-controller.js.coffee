App.HostVolumesController = Em.ObjectController.extend
  actions:
    new: ->
      @transitionToRoute 'host.volumes-new'

    remove: (volumes) ->
      host = @get 'model'

      fns = volumes.map (volume) ->
        (cb) ->
          volume.ajax('DELETE').then ->
            cb(null)
          , (jqxhr) ->
            if errMsg = jqxhr.responseJSON?.error_description
              App.snackbar.post(errMsg.capitalize())
            cb(jqxhr)

      async.series fns, (err) ->
        host.get('volumes').reload()

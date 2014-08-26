App.HostRegistriesController = Em.ObjectController.extend
  actions:
    add: ->
      @transitionToRoute 'host.registries-new'

    remove: (registries) ->
      host = @get 'model'

      fns = registries.map (registry) ->
        (cb) ->
          registry.ajax('DELETE').then ->
            cb(null)
          , (jqxhr) ->
            if errMsg = jqxhr.responseJSON?.error_description
              App.snackbar.post(errMsg.capitalize())
            cb(jqxhr)

      async.series fns, (err) ->
        host.get('registries').reload()

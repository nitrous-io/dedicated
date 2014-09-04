App.HostRegistriesController = Em.ObjectController.extend
  removing: false

  clear: ->
    @setProperties
      removing: false

  actions:
    add: ->
      @transitionToRoute 'host.registries-new'

    remove: (registries) ->
      @set 'removing', true
      host = @get 'model'

      fns = registries.map (registry) ->
        (cb) ->
          registry.ajax('DELETE').then ->
            cb(null)
          , (jqxhr) ->
            if errMsg = jqxhr.responseJSON?.error_description
              App.snackbar.post(errMsg.capitalize())
            cb(jqxhr)

      async.series fns, (err) =>
        host.get('registries').reload()
        @set 'removing', false

App.ContainerKeysController = Em.ObjectController.extend
  removing: false

  clear: ->
    @setProperties
      removing: false

  actions:
    add: ->
      @transitionToRoute 'container.keys-new'

    remove: (keys) ->
      @set 'removing', true
      host = @get 'model'

      fns = keys.map (key) ->
        (cb) ->
          key.ajax('DELETE').then ->
            cb(null)
          , (jqxhr) ->
            if errMsg = jqxhr.responseJSON?.error_description
              App.snackbar.post(errMsg.capitalize())
            cb(jqxhr)

      async.series fns, (err) =>
        host.get('keys').reload()
        @set 'removing', false

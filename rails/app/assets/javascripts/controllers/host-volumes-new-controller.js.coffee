App.HostVolumesNewController = Em.ObjectController.extend
  name: null
  slug: null
  saving: false

  clear: ->
    @setProperties
      name: null
      slug: null
      saving: false

  actions:
    cancel: ->
      @transitionToRoute 'host.volumes'
      false

    create: ->
      @set 'saving', true
      host = @get 'model'
      volume = App.Volume.create
        hostId: host.get('id')
        name: @get('name')
        slug: @get('slug')
      volume.save().then =>
        @transitionToRoute 'host.volumes'
        @set 'saving', false
      , (jqxhr) =>
        errMsg = jqxhr.responseJSON?.error_description
        App.snackbar.post(errMsg?.capitalize() || 'Unknown Error')
        @set 'saving', false
      false

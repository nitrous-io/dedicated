App.ContainerKeysNewController = Em.ObjectController.extend
  key: null
  saving: false

  clear: ->
    @setProperties
      key: null
      saving: false

  actions:
    cancel: ->
      @transitionToRoute 'container.keys'
      false

    create: ->
      @set 'saving', true
      container = @get 'model'
      pubKey = App.PublicKey.create
        containerId: container.get('id')
        key: @get 'key'
      pubKey.save().then =>
        @transitionToRoute 'container.keys'
        @set 'saving', false
      , (jqxhr) =>
        errMsg = jqxhr.responseJSON?.error_description
        App.snackbar.post(errMsg?.capitalize() || 'Unknown Error')
        @set 'saving', false
      false

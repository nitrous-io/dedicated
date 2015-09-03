App.ContainerKeysNewController = Em.ObjectController.extend
  containerBinding: Em.Binding.oneWay 'model.container'

  key: null
  saving: false
  record: Em.computed.alias 'model'

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
      container = @get 'record'
      pubKey = App.UserKey.create
        containerId: container.get('id')
        key: @get 'key'
      pubKey.save().then =>
        @transitionToRoute 'container.keys'
        @set 'saving', false
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to add public key!')
        @set 'saving', false
      false

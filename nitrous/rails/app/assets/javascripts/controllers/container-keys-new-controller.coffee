App.ContainerKeysNewController = Em.ObjectController.extend
  key: null
  saving: false
  indexRoute: 'container.keys'
  record: Em.computed.alias 'model'

  clear: ->
    @setProperties
      key: null
      saving: false

  actions:
    cancel: ->
      @transitionToRoute(@get('indexRoute'))
      false

    create: ->
      @set 'saving', true
      container = @get 'record'
      pubKey = App.UserKey.create
        containerId: container.get('id')
        key: @get 'key'
      pubKey.save().then =>
        @transitionToRoute(@get('indexRoute'))
        @set 'saving', false
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to add public key!')
        @set 'saving', false
      false

App.HostRegistriesNewController = Em.ObjectController.extend
  url: null
  username: null
  email: null
  password: null
  saving: false

  clear: ->
    @setProperties
      url: null
      username: null
      email: null
      password: null
      saving: false

  actions:
    cancel: ->
      @transitionToRoute 'host.registries'
      false

    create: ->
      @set 'saving', true
      host = @get 'model'
      registry = App.Registry.create
        hostId: host.get('id')
        url: @get('url')
        username: @get('username')
        email: @get('email')
        password: @get('password')
      registry.save().then =>
        @transitionToRoute 'host.registries'
        @set 'saving', false
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to connect!')
        @set 'saving', false
      false

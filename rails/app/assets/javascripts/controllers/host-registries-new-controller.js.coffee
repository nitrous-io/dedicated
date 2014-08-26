App.HostRegistriesNewController = Em.ObjectController.extend
  url: null
  username: null
  email: null
  password: null
  invalidField: null
  saving: false

  clear: ->
    @setProperties
      url: null
      username: null
      email: null
      password: null
      invalidField: null
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
        host.get('registries').reload()
        @transitionToRoute 'host.registries'
        @set 'saving', false
      , (jqxhr) =>
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
          App.snackbar.post(errMsg.capitalize())
        @set 'saving', false
      false

transformEnvHashToArray = (hash) ->
  envVars = []
  if hash
    for own key, value of hash
      envVars.push("#{key}=#{value}") if value
  envVars

App.ContainerPortsController = Lib.ContainerConfigController.extend
  saving: (->
    m = @get 'model'
    m && m.get('underReconfig')
  ).property 'model', 'model.underReconfig'
  loaded: false
  newPreviewPort: null
  setPreviewPort: false
  removePreviewPort: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      loaded: false
      newPreviewPort: null
      setPreviewPort: false
      removePreviewPort: false

  loadConfig: ->
    container = @get 'model'
    container.loadConfig().then (config) =>
      @set 'portMappings', Lib.PortMappings.create
        configuredPorts: config.config_port_bindings
        livePorts: config.port
      @set 'loaded', true
    , (jqxhr) ->
      App.postError(jqxhr.responseJSON, 'Failed to fetch configuration!')

  actions:
    add: ->
      containerPort = @get 'containerPort'
      if @get('setPreviewPort')
        @set('newPreviewPort', containerPort)
      @set 'setPreviewPort', false

      @set('publicPort', true) # all port mappings are public
      @send('addPortMapping') # in Lib.ContainerConfigController

    apply: ->
      return if @get 'saving'
      container = @get 'model'
      prevState = @get 'state'
      @set 'state', 'reconfiguring'

      events = App.get 'eventsSocket'

      reconfigureComplete = =>
        @set 'state', prevState
        @set 'removePreviewPort', false
        @set 'newPreviewPort', null
        events.off 'container.reconfigured', onReconfigured
        events.off 'container.reconfigure_failed', onReconfigureFailed

      onReconfigured = (data) =>
        reconfigureComplete()
        config = data.config
        container.setConfig(config)
        @set 'portMappings', Lib.PortMappings.create
          configuredPorts: config.config_port_bindings
          livePorts: config.port
        App.snackbar.post 'Port changes applied!'

      onReconfigureFailed = (data) =>
        reconfigureComplete()
        App.postError(data.error, "Failed to apply port changes!")

      events.on 'container.reconfigured', onReconfigured
      events.on 'container.reconfigure_failed', onReconfigureFailed

      env = Em.copy container.get('config.env')
      if newPreviewPort = @get('newPreviewPort')
        env['NITROUS_PREVIEW_PORT'] = newPreviewPort
      if @get 'removePreviewPort'
        delete env['NITROUS_PREVIEW_PORT']

      container.saveConfig(
        port: @get 'portMappings.serialized'
        env: transformEnvHashToArray env
      ).then =>
        App.snackbar.post 'Applying port changes...'
      , (jqxhr) ->
        reconfigureComplete()
        App.postError(jqxhr.responseJSON, "Failed to apply port changes!")

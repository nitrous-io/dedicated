readOnlyEnvKeys = ['__HOST']

App.ContainerConfigController = Lib.ContainerConfigController.extend
  saving: (->
    m = @get 'model'
    m && m.get('underReconfig')
  ).property 'model', 'model.underReconfig'
  loaded: false
  closeApplyDialog: false
  envVarHashRO: null # object (hash) - readonly

  clear: ->
    @_super.apply this, arguments
    @setProperties
      loaded: false
      closeApplyDialog: false
      envVarHashRO: Em.Object.create()

  hostVolumesEmpty: Em.computed.empty('host.volumes')

  envVarsRO: (->
    @transformEnvHashToArray(@get 'envVarHashRO')
  ).property 'envVarHashRO'

  loadConfig: ->
    container = @get 'model'
    container.loadConfig().then (config) =>
      @updateConfigForm(config)
      @set 'loaded', true
    , (jqxhr) ->
      App.postError(jqxhr.responseJSON, "Failed to fetch configuration!")

  updateConfigForm: (config) ->
    @set 'portMappings', Lib.PortMappings.create
      ports: config.config_port_bindings || config.port
      
    if config.env
      [mutableEnv, readOnlyEnv] = filterEnvVarHash(config.env)
      @set 'envVarHash', mutableEnv
      @set 'envVarHashRO', readOnlyEnv

    if config.volume
      @set 'volumeMounts', config.volume

    if config.privileged?
      @set 'privileged', config.privileged

    if config.publish_all?
      @set 'publishAll', config.publish_all

  actions:
    apply: ->
      return if @get 'saving'

      container = @get('model')
      prevState = @get 'state'
      @set 'state', 'reconfiguring'

      events = App.get 'eventsSocket'

      reconfigureComplete = =>
        @set 'state', prevState
        events.off 'container.reconfigured', onReconfigured
        events.off 'container.reconfigure_failed', onReconfigureFailed

      onReconfigured = (data) =>
        reconfigureComplete()
        @updateConfigForm(container.setConfig(data.config))
        App.snackbar.post 'New configuration applied!'

      onReconfigureFailed = (data) =>
        reconfigureComplete()
        App.postError(data.error, "Failed to apply new configuration!")

      events.on 'container.reconfigured', onReconfigured
      events.on 'container.reconfigure_failed', onReconfigureFailed

      container.saveConfig(
        env: @get 'envVars'
        volume: @get 'volumeMounts'
        port: @get 'portMappings.rawPorts'
        privileged: @get 'privileged'
        publish_all: @get 'publishAll'
      ).then(=>
        App.snackbar.post 'Applying new configuration...'
        @set 'closeApplyDialog', true
      , (jqxhr) =>
        @set 'state', prevState
        @set 'closeApplyDialog', true
        container.set 'underReconfig', false
        App.postError(jqxhr.responseJSON, "Failed to apply new configuration!")
      )

filterEnvVarHash = (envVarHash) ->
  mutable = envVarHash
  readonly = {}
  for key in readOnlyEnvKeys
    if val = mutable[key]?
      readonly[key] = val
      delete mutable[key]
  [mutable, readonly]

readOnlyEnvKeys = ['__HOST']
readOnlyVolumeSuffixes = [':/nitrous/ssh:ro']

App.ContainerConfigController = Lib.ContainerConfigController.extend
  saving: false
  loaded: false
  closeApplyDialog: false
  envVarHashRO: null # object (hash) - readonly
  volumeMountsRO: null # array - readonly

  clear: ->
    @_super.apply this, arguments
    @setProperties
      saving: false
      loaded: false
      closeApplyDialog: false
      envVarHashRO: Em.Object.create()
      volumeMountsRO: Em.A()

  hostVolumesEmpty: Em.computed.empty('host.volumes')

  envVarsRO: (->
    @transformEnvHashToArray(@get 'envVarHashRO')
  ).property 'envVarHashRO'

  loadConfig: ->
    container = @get 'model'
    container.loadConfig().then (config) =>
      if config.env
        [mutableEnv, readOnlyEnv] = filterEnvVarHash(config.env)
        @set 'envVarHash', mutableEnv
        @set 'envVarHashRO', readOnlyEnv

      if config.volume
        [mutableVol, readOnlyVol] = filterVolumeMounts(config.volume)
        @set 'volumeMounts', mutableVol
        @set 'volumeMountsRO', readOnlyVol

      if config.port
        @set 'portBindings', config.port

      if config.publish_all
        @set 'publishAll', config.publish_all

      @set 'loaded', true
    , (jqxhr) ->
      App.postError(jqxhr.responseJSON, "Failed to fetch configuration!")

  actions:
    apply: ->
      return if @get 'saving'
      @set 'saving', true
      container = @get('model')
      container.saveConfig(
        env: @get 'envVars'
        volume: @get 'volumeMounts'
        port: @get 'portBindings'
        publish_all: @get 'publishAll'
      ).then(=>
        App.snackbar.post 'New configuration applied!'
        @set 'closeApplyDialog', true
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to apply new configuration!")
      ).finally =>
        @set 'saving', false

filterEnvVarHash = (envVarHash) ->
  mutable = envVarHash
  readonly = {}
  for key in readOnlyEnvKeys
    if val = mutable[key]?
      readonly[key] = val
      delete mutable[key]
  [mutable, readonly]

filterVolumeMounts = (volumeMounts) ->
  mutable = Em.A()
  readonly = Em.A()
  for vol, i in volumeMounts
    filtered = false
    for suffix in readOnlyVolumeSuffixes
      suffixIdx = vol.indexOf(suffix)
      if suffixIdx != -1 && suffixIdx == (vol.length - suffix.length)
        readonly.pushObject(vol)
        filtered = true
        continue
    mutable.pushObject(vol) unless filtered
  [mutable, readonly]

readOnlyEnvKeys = ['__HOST']
readOnlyVolumeSuffixes = [':/nitrous/ssh:ro']

App.ContainerConfigController = Lib.ContainerConfigController.extend
  saving: false
  loaded: false
  envVarHashRO: null # object (hash) - readonly
  volumeMountsRO: null # array - readonly

  clear: ->
    @_super.apply this, arguments
    @set 'envVarHashRO', Em.Object.create()
    @set 'volumeMountsRO', Em.A()

  hostVolumesIsEmpty: Em.computed.empty('host.volumes')

  envVarsRO: (->
    @transformEnvHashToArray(@get 'envVarHashRO')
  ).property 'envVarHashRO'

  loadFields: ->
    container = @get 'model'
    container.loadConfig().then (config) =>
      [mutableEnv, readOnlyEnv] = filterEnvVarHash(transformEnvArrayToHash(config.env))
      @set 'envVarHash', mutableEnv
      @set 'envVarHashRO', readOnlyEnv

      [mutableVol, readOnlyVol] = filterVolumeMounts(config.volume)
      @set 'volumeMounts', mutableVol
      @set 'volumeMountsRO', readOnlyVol

      @set 'portBindings', config.port
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
      ).then(->
        App.snackbar.post 'New configuration applied!'
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to apply new configuration!")
      ).finally =>
        @set 'saving', false

transformEnvArrayToHash = (envVars) ->
  hash = {}
  for v in envVars
    m = v.match /^([^=]+)=(.*)$/
    key = m[1]
    val = m[2]
    hash[key] = val
  hash

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
      if vol.indexOf(suffix) == vol.length - suffix.length
        readonly.pushObject(vol)
        filtered = true
        continue
    mutable.pushObject(vol) unless filtered
  [mutable, readonly]

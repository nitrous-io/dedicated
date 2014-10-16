Lib.ContainerConfigController = Em.ObjectController.extend
  volumeMounts: null # array
  selectedVolumeId: null
  volumeMountpoint: null

  portBindings: null # array
  publicPort: false
  hostPort: null
  containerPort: null
  publishAll: true

  envVarHash: null # object (hash)
  envKey: null
  envValue: null

  clear: ->
    @setProperties
      volumeMounts: Em.A()
      selectedVolumeId: null
      volumeMountpoint: null

      portBindings: Em.A()
      publicPort: false
      hostPort: null
      containerPort: null
      publishAll: true

      envVarHash: Em.Object.create()
      envKey: null
      envValue: null

  selectedVolume: (->
    volumes = @get 'host.volumes'
    id = @get 'selectedVolumeId'
    if volumes && id
      volumes.findBy 'id', parseInt(id, 10)
  ).property 'host.volumes.@each', 'selectedVolumeId'

  envVars: (->
    @transformEnvHashToArray(@get 'envVarHash')
  ).property 'envVarHash'

  transformEnvHashToArray: (hash) ->
    envVars = []
    if hash
      for own key, value of hash
        envVars.push("#{key}=#{value}") if value
    envVars

  actions:
    addVolumeMount: ->
      volumeMounts = @get 'volumeMounts'
      vol = @get 'selectedVolume.slug'
      if mountpoint = @get 'volumeMountpoint'
        vol += ':' + mountpoint
      unless volumeMounts.contains(vol)
        volumeMounts.pushObject(vol)
      @set 'volumeMountpoint', null

    removeVolumeMount: (vol) ->
      @get('volumeMounts').removeObject(vol)

    addPortBinding: ->
      portBindings = @get 'portBindings'
      publicPort = if @get('publicPort')
        "public"
      else
        "private"
      hostPort = @get('hostPort') ? ''
      containerPort = @get 'containerPort'
      pb = [publicPort, hostPort, containerPort].join(':')
      unless portBindings.contains(pb)
        portBindings.pushObject(pb)
      @setProperties
        hostPort: null
        containerPort: null

    removePortBinding: (pb) ->
      @get('portBindings').removeObject(pb)

    addEnvVar: ->
      envVarHash = @get 'envVarHash'
      envKey = @get 'envKey'
      envValue = @get 'envValue'
      envVarHash[envKey] = envValue
      @notifyPropertyChange 'envVarHash'
      @setProperties
        envKey: null
        envValue: null

    removeEnvVar: (env) ->
      key = env.substring(0, env.indexOf('='))
      @get('envVarHash')[key] = null
      @notifyPropertyChange 'envVarHash'

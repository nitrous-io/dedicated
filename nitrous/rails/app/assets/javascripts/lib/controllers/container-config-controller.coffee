#= require ../port-mappings

Lib.ContainerConfigController = Em.ObjectController.extend
  volumeMounts: null # array
  selectedVolumeId: null
  volumeMountpoint: null

  portMappings: null
  publicPort: false
  hostPort: null
  containerPort: null
  protocol: 'tcp'

  privileged: false
  publishAll: true

  envVarHash: null # object (hash)
  envKey: null
  envValue: null

  protocolOptions: [
    {
      id: 'tcp'
      displayName: 'TCP'
    },
    {
      id: 'udp'
      displayName: 'UDP'
    },
    {
      id: 'both'
      displayName: 'Both'
    }
  ]

  clear: ->
    @setProperties
      volumeMounts: Em.A()
      selectedVolumeId: null
      volumeMountpoint: null

      portMappings: Lib.PortMappings.create()

      publicPort: false
      hostPort: null
      containerPort: null
      protocol: 'tcp'

      privileged: false
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
    envVars.sort (a,b)->
      a.toLowerCase().localeCompare(b.toLowerCase())

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

    addPortMapping: ->
      portMappings = @get('portMappings')
      scope = if @get('publicPort')
        'public'
      else
        'private'
      hostPort = @get('hostPort') ? ''
      containerPort = @get('containerPort')
      protocol = @get('protocol')

      postErr = (hostPort, containerPort, protocol) ->
        msg = if hostPort
          "Cannot add port mapping for #{hostPort} → #{containerPort}/#{protocol} because a conflicting mapping exists."
        else
          "Cannot add port mapping for container port #{containerPort}/#{protocol} because a conflicting mapping exists."
        App.snackbar.post(msg, sticky: false)

      if protocol == 'both'
        if !portMappings.add(scope, hostPort, containerPort, 'tcp')
          postErr(hostPort, containerPort, 'tcp')
          return
        if !portMappings.add(scope, hostPort, containerPort, 'udp')
          postErr(hostPort, containerPort, 'udp')
          return
      else
        if !portMappings.add(scope, hostPort, containerPort, protocol)
          postErr(hostPort, containerPort, protocol)
          return

      @setProperties
        hostPort: null
        containerPort: null

    removePortMapping: (mapping) ->
      @get('portMappings').remove(mapping)

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

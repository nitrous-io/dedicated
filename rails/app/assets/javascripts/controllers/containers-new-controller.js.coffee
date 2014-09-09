App.ContainersNewController = Em.Controller.extend
  selectedHostSlug: null
  name: null
  slug: null
  invalidField: null

  volumeMounts: null
  selectedVolumeId: null
  volumeMountpoint: null

  portBindings: null
  publicPort: false
  hostPort: null
  containerPort: null
  publishAll: true

  envVarHash: null
  envKey: null
  envValue: null

  saving: false

  clear: ->
    @setProperties
      selectedHostSlug: null
      name: null
      slug: null
      invalidField: null

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

      saving: false

  hostImagesIsEmpty: Em.computed.empty('selectedHost.images')
  hostVolumesIsEmpty: Em.computed.empty('selectedHost.images')

  envVars: (->
    return unless envVarHash = @get 'envVarHash'
    envVars = []
    for own key, value of envVarHash
      envVars.push("#{key}=#{value}") if value
    envVars
  ).property 'envVarHash'

  selectedHost: (->
    if slug = @get 'selectedHostSlug'
      App.get('hosts').findBy 'slug', slug
  ).property 'selectedHostSlug', 'App.hosts.@each'

  selectedVolume: (->
    volumes = @get 'selectedHost.volumes'
    id = @get 'selectedVolumeId'
    if volumes && id
      volumes.findBy 'id', parseInt(id, 10)
  ).property 'selectedHost.volumes.@each', 'selectedVolumeId'

  didChangeSelectedHost: (->
    @set 'volumeMounts', Em.A()
  ).observes 'selectedHost'

  actions:
    addVolumeMount: ->
      volumeMounts = @get 'volumeMounts'
      vol = @get 'selectedVolume.slug'
      if mountpoint = @get 'volumeMountpoint'
        vol += ':' + mountpoint
      unless volumeMounts.contains(vol)
        volumeMounts.pushObject(vol)
      @set 'volumeMountpoint', null

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

    addEnvVar: ->
      envVarHash = @get 'envVarHash'
      envKey = @get 'envKey'
      envValue = @get 'envValue'
      envVarHash[envKey] = envValue
      @notifyPropertyChange 'envVarHash'
      @setProperties
        envKey: null
        envValue: null

    removeVolumeMount: (vol) ->
      @get('volumeMounts').removeObject(vol)

    removePortBinding: (pb) ->
      @get('portBindings').removeObject(pb)

    removeEnvVar: (env) ->
      key = env.substring(0, env.indexOf('='))
      @get('envVarHash')[key] = null
      @notifyPropertyChange 'envVarHash'

    cancel: ->
      @transitionToRoute 'home.index'

    create: (image) ->
      @set 'saving', true
      container = App.Container.create
        name: @get('name')
        slug: @get('slug')
        image: image
      container.save(host: @get('selectedHostSlug'), volume: @get('volumeMounts'), env: @get('envVars'), port: @get('portBindings')).then =>
        App.get('containers').reload().then =>
          @transitionToRoute 'container.index', container.get 'slug'
          @set 'saving', false
      , (jqxhr) =>
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'error', errMsg
          @set 'invalidField', errMsg.match(/^\w+/)[0]
          App.snackbar.post(errMsg.capitalize())
        @set 'saving', false

App.ContainersNewController = Lib.ContainerConfigController.extend
  queryParams: ['preselectHost']
  preselectHost: null

  selectedHostSlug: null
  name: null
  slug: null
  invalidField: null

  image: null
  saving: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      selectedHostSlug: null
      name: null
      slug: null
      invalidField: null

      image: null
      saving: false

  hostImagesEmpty: Em.computed.empty('selectedHost.images')
  hostVolumesEmpty: Em.computed.empty('selectedHost.volumes')

  selectedHost: (->
    if slug = @get 'selectedHostSlug'
      App.get('hosts').findBy 'slug', slug
  ).property 'selectedHostSlug', 'App.hosts.@each'

  host: Em.computed.alias 'selectedHost'

  didChangeSelectedHost: (->
    @set 'volumeMounts', Em.A()
  ).observes 'selectedHost'

  handleErrorResponse: (jqxhr) ->
    errMsg = null
    if errMsg = jqxhr.responseJSON?.error_description
      @set 'error', errMsg
      @set 'invalidField', errMsg.match(/^\w+/)[0]
    App.postError(jqxhr.responseJSON, 'Failed to create container!')
    @set 'saving', false

  actions:
    cancel: ->
      if slug = @get('preselectHost')
        @transitionToRoute 'host.index', slug
      else
        @transitionToRoute 'home.index'

    create: (repository, isNitrousBaseImage) ->
      @set 'saving', true
      events = App.get 'eventsSocket'

      unless repository.match /^.*:.+$/
        repository = repository.replace /:?$/, ':latest'
      name = @get 'name'
      slug = @get 'slug'

      # pull image
      App.Image.pull(@get('selectedHost'), repository, isNitrousBaseImage).then (image) =>
        @set 'image', image

        onImagePullFailed = (data) =>
          imageData = data.image
          return unless imageData.id == image.get('id')
          events.off 'image.pull_failed', onImagePullFailed
          events.off 'image.pulled', onImagePulled
          @set 'image', null
          errMsg = null
          if data.progress
            progress = JSON.parse(progress)
            errMsg = progress.errorDetail?.message
          App.postError(errMsg, 'Failed to pull image!')
          @set 'saving', false

        onImagePulled = (data) =>
          imageData = data.image
          return unless imageData.id == image.get('id')
          events.off 'image.pull_failed', onImagePullFailed
          events.off 'image.pulled', onImagePulled

          # create container
          container = App.Container.create
            name: name
            slug: slug
            image: repository

          container.save(host: @get('selectedHostSlug'), volume: @get('volumeMounts'), env: @get('envVars'), port: @get('portBindings'), publish_all: @get('publishAll')).then =>
            @transitionToRoute 'container.index', slug
            App.snackbar.post("Your container “#{name}” was created successfully.")
            @set 'saving', false
          , (jqxhr) => # container.save
            @handleErrorResponse(jqxhr)

        events.on 'image.pull_failed', onImagePullFailed
        events.on 'image.pulled', onImagePulled
      , (jqxhr) => # App.Image.pull
        @handleErrorResponse(jqxhr)

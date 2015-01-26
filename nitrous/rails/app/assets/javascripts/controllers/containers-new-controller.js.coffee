App.ContainersNewController = Lib.ContainerConfigController.extend
  queryParams: ['preselectHost']
  preselectHost: null

  selectedHostSlug: null
  name: null
  slug: null
  error: null
  invalidField: null

  image: null
  privileged: false
  saving: false
  pullingImage: false

  selectedOption: null
  selectedTemplate: null

  stackTemplates: (->
    App.get('templates')?.filterBy('type', 'stack')
  ).property 'App.templates.@each'

  basicTemplates: (->
    App.get('templates')?.filterBy('type', 'basic')
  ).property 'App.templates.@each'

  advancedTemplates: (->
    App.get('templates')?.filterBy('type', 'advanced')
  ).property 'App.templates.@each'

  clear: ->
    @_super.apply this, arguments
    @setProperties
      selectedHostSlug: null
      name: null
      slug: null
      error: null
      invalidField: null

      image: null
      saving: false
      pullingImage: false

      selectedTemplate: App.get('templates.firstObject')
      selectedOption: null

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

  createContainer: (name, slug, repository, template) ->
    # create container
    container = App.Container.create
      name: name
      slug: slug
      image: repository
      template: template

    container.save(
      host: @get 'selectedHostSlug'
      volume: @get 'volumeMounts'
      env: @get 'envVars'
      port: @get 'portBindings'
      privileged: @get 'privileged'
      publish_all: @get 'publishAll'
    ).then =>
      @transitionToRoute 'container.index', slug
      App.snackbar.post("Your new container “#{name}” was created successfully.")
      @set 'saving', false
    , (jqxhr) => # container.save
      @handleErrorResponse(jqxhr)

  pullImage: (repository, isNitrousBaseImage, callback) ->
    events = App.get 'eventsSocket'
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

        callback()

      events.on 'image.pull_failed', onImagePullFailed
      events.on 'image.pulled', onImagePulled
    , (jqxhr) => # App.Image.pull
      @handleErrorResponse(jqxhr)

  actions:
    cancel: ->
      if slug = @get('preselectHost')
        @transitionToRoute 'host.index', slug
      else
        @transitionToRoute 'home.index'

    create: (repository, isNitrousBaseImage) ->
      @set 'saving', true

      name = @get 'name'
      slug = @get 'slug'

      @set 'pullingImage', true

      if @get('selectedTemplate') && !@get('selectedOption')
        img = @get('selectedTemplate.images.firstObject')
        template = @get('selectedTemplate.slug')
        @pullImage img, isNitrousBaseImage, =>
          @set 'pullingImage', false
          @createContainer(name, slug, img, template)
        return

      # pull regular image
      unless repository.match /^.*:.+$/
        repository = repository.replace /:?$/, ':latest'
      @pullImage repository, isNitrousBaseImage, =>
        @set 'pullingImage', false
        @createContainer(name, slug, repository)

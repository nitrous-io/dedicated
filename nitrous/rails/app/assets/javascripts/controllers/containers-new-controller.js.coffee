App.ContainersNewController = Lib.ContainerConfigController.extend
  queryParams: ['preselectHost']
  preselectHost: null

  selectedHostSlug: null
  name: null
  error: null
  invalidField: null

  image: null
  privileged: false
  saving: false
  pullingImage: false
  ideStarting: false

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

  createContainer: (name, repository, template) ->
    # create container
    container = App.Container.create
      name: name
      image: repository
      template: template

    container.save(
      host: @get 'selectedHostSlug'
      volume: @get 'volumeMounts'
      env: @get 'envVars'
      port: @get 'portMappings.rawPorts'
      privileged: @get 'privileged'
      publish_all: @get 'publishAll'
    ).then =>
      if App.get('user.advanced')
        @transitionToRoute 'container.index', container.get('slug')
        App.snackbar.post("Your new container “#{name}” was created successfully.")
      else
        @set('ideStarting', true)
        window.location.href = container.get('ideURL')
      @set 'saving', false
    , (jqxhr) => # container.save
      @handleErrorResponse(jqxhr)

  pullImage: (repository, callback) ->
    @set 'pullingImage', true
    events = App.get 'eventsSocket'
    App.Image.pull(@get('selectedHost'), repository, true).then (image) =>
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
        @set 'pullingImage', false
        @set 'saving', false

      onImagePulled = (data) =>
        imageData = data.image
        return unless imageData.id == image.get('id')
        events.off 'image.pull_failed', onImagePullFailed
        events.off 'image.pulled', onImagePulled
        callback()
        @set 'pullingImage', false

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

    create: (repository) ->
      @set 'saving', true

      name = @get 'name'
      slug = @get 'slug'
      selectedOption = @get 'selectedOption'
      selectedTemplate = @get 'selectedTemplate'

      if selectedOption == 'host-image'
        # image already on host - no pull needed
        @createContainer(name, repository)
      else if selectedOption == 'custom-image'
        # custom image
        unless repository.match /^.*:.+$/
          repository = repository.replace /:?$/, ':latest'
        @pullImage repository, =>
          @createContainer(name, repository)
      else if selectedTemplate
        # update template
        img = selectedTemplate.get 'image'
        template = selectedTemplate.get 'slug'
        @pullImage img, =>
          @createContainer(name, img, template)

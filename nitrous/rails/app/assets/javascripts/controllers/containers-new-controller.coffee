App.ContainersNewController = Lib.ContainerConfigController.extend
  queryParams: ['preselectHost', 'runCreate', 'selectedTemplateSlug', 'autofillName']
  preselectHost: null
  # Flag to indicate whether we allow a container to be created immediately upon
  # entering this route. This prevents unintended creates when the browser
  # "Back" button is used.
  runCreate: false
  selectedTemplateSlug: null
  autofillName: null

  selectedHostSlug: null
  name: null
  error: null
  invalidField: null

  image: null
  privileged: false
  savingBinding: 'App.containerSaving'
  pullingImage: false
  createdContainer: null
  openingIDE: false

  selectedOption: null
  selectedTemplate: null

  nameFieldEdited: false

  gitRepositories: []
  selectedGitRepository: null

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
    selectedTemplate = null
    if sl = @get 'selectedTemplateSlug'
      selectedTemplate = App.get('templates').findBy 'slug', sl
    selectedTemplate ||= App.get 'templates.firstObject'

    @setProperties
      selectedHostSlug: if App.get('singleHost') then App.get('hosts.firstObject.slug') else null
      name: @get 'autofillName'
      error: null
      invalidField: null

      image: null
      saving: false
      pullingImage: false
      createdContainer: null
      openingIDE: false

      selectedTemplate: selectedTemplate
      selectedOption: null

      nameFieldEdited: @get('autofillName') != null

      autofillName: null
      selectedTemplateSlug: null

      selectedGitRepository: null

    @set 'gitRepositories', App.Repository.fetch().value

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
    container = App.Container.create
      name: name
      image: repository
      template: template

    container.save(
      host: @get 'selectedHostSlug'
      volume: @get 'volumeMounts'
      env: @get 'envVars'
      port: @get 'portMappings.serialized'
      privileged: @get 'privileged'
      publish_all: @get 'publishAll'
      git_url: @get 'selectedGitRepository'
    ).then =>
      App.snackbar.post("Your new container “#{name}” was created successfully.", sticky: true)
      @set 'createdContainer', container
      @set 'saving', false
      p = {}
      p[Mp.PropertyImagesList] = template or repository
      Mp.unionUser(p)
      App.set 'onboardingScreen', null
      if App.get 'onboardingIDEOpen'
        @send 'openIDE'
    , (jqxhr) => # container.save
      @handleErrorResponse(jqxhr)

  canCreateContainer: (callback) ->
    App.User.adapter.ajax('GET', "/hosts/#{@get('selectedHostSlug')}/can_create_containers").done((d) =>
      if d.can_create
        return callback?()

      @set 'saving', false
      if sub = @get 'selectedHost.subscription.slug'
        @transitionToRoute 'subscriptions.upgrade', sub, queryParams: { exceededContainerLimit: true, selectedTemplate: @get('selectedTemplate.slug'), containerName: @get('name') }
      else
        App.postError "You have exceeded your container limit on #{@get('selectedHost.name')}"
    ).fail (jqxhr) =>
      @set 'saving', false
      App.postError(jqxhr.responseJSON, "Failed to create container!")

  pullImage: (repository, callback) ->
    @set 'pullingImage', true
    events = App.get 'eventsSocket'
    App.Image.pull(@get('selectedHostSlug'), repository, true).then (image) =>
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

  reflectSaving: (->
    return if @get 'saving'
    App.set 'hostCreatingContainer', null
  ).observes 'saving'

  actions:
    showContainer: ->
      return unless @get 'createdContainer'
      @transitionToRoute 'container.index', @get 'createdContainer.slug'

    openIDE: ->
      return unless @get 'createdContainer' || @get('openingIDE')
      @set 'openingIDE', true
      window.location.href = @get 'createdContainer.ideURL'

    cancel: ->
      if slug = @get('preselectHost')
        @transitionToRoute 'host.index', slug
      else
        @transitionToRoute 'home.index'

    create: (repository) ->
      @set 'saving', true
      App.set 'hostCreatingContainer', @get 'selectedHostSlug'
      App.set 'user.firstTime', false

      if App.get('onboardingScreen')
        App.set 'onboardingScreen', 'thank-you'

      name = @get 'name'
      selectedOption = @get 'selectedOption'
      selectedTemplate = @get 'selectedTemplate'

      if selectedOption == 'host-image'
        # image already on host - no pull needed
        @createContainer(name, repository)
      else if selectedOption == 'custom-image'
        # custom image
        unless repository.match /^.*:.+$/
          repository = repository.replace /:?$/, ':latest'
        @canCreateContainer =>
          @pullImage repository, =>
            @createContainer(name, repository)
      else if selectedTemplate
        # update template
        img = selectedTemplate.get 'image'
        template = selectedTemplate.get 'slug'
        @canCreateContainer =>
          @pullImage img, =>
            @createContainer(name, img, template)

    createWorkspaceFirst: (repository) ->
      # Save data required to create container after creating workspace.
      App.set 'pendingContainerData', Em.Object.create
        name: @get 'name'
        repository: repository
        selectedOption: @get 'selectedOption'
        selectedTemplate: @get 'selectedTemplate'
        envVars: @get 'envVars'
        portMappings: @get 'portMappings'
        privileged: @get 'privileged'
        publishAll: @get 'publishAll'

      @transitionToRoute 'hosts.new', queryParams: { pendingContainer: true }

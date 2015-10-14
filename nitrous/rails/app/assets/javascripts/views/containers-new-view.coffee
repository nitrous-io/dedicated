#= require ../mixins/inject-terminal-iframe-mixin

optionHostImage = 'host-image'
optionCustomImage = 'custom-image'
heightPadding = 16
screens = ['image', 'config', 'progress']

unloadMessage = "WARNING: Leaving the app will cancel container creation."

App.ContainersNewView = Lib.ContainerConfigView.extend App.InjectTerminalIframeMixin,
  elementId: 'containers-new-view'
  classNames: ['ui', 'card']

  selectedHostImage: null
  customImage: null

  selectedOptionBinding: "controller.selectedOption"
  selectedTemplateBinding: "controller.selectedTemplate"
  selectedGitRepositoryBinding: "controller.selectedGitRepository"

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')

  currentScreenIndex: 0
  showImageScreen: Em.computed.equal('currentScreenIndex', screens.indexOf('image'))
  showConfigScreen: Em.computed.equal('currentScreenIndex', screens.indexOf('config'))
  showProgressScreen: Em.computed.equal('currentScreenIndex', screens.indexOf('progress'))

  showLog: false

  image: (->
    option = @get 'selectedOption'
    switch option
      when optionHostImage
        unless @get 'controller.hostImagesEmpty'
          @get 'selectedHostImage'
      when optionCustomImage
        @get 'customImage'
  ).property 'selectedOption', 'selectedHostImage', 'customImage', 'controller.hostImagesEmpty'

  firstScreenButtonLabel: (->
    if App.get('user.advanced') || App.get('noHost')
      'Next'
    else
      'Create'
  ).property 'App.user.advanced', 'App.noHost'

  advancedButtonLabel: (->
    if App.get('noHost')
      'Next'
    else
      'Create'
  ).property 'App.noHost'

  createButtonDisabled: (->
    !(@get('image') || @get('selectedTemplate'))
  ).property 'image', 'selectedTemplate'

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.image.progress.progressDetail.start')

  # We use a property + observer pair here so that we can notify the user when
  # their selected host is no longer running.
  availableHosts: null
  availableHostsDidChange: (->
    hosts = App.get('hosts').filter (host)->
      host.get('isRunning')

    if selectedHost = @get('controller.selectedHostSlug')
      h = hosts.find (host)->
        host.get('slug') == selectedHost
      unless h
        App.snackbar.post "'#{selectedHost}' is no longer running."

    @set('availableHosts', hosts)
  ).observes('App.hosts.@each.isRunning').on('init')

  preselectHostDidChange: (->
    if slug = @get('controller.preselectHost')
      @set 'controller.selectedHostSlug', slug
  ).observes 'controller.preselectHost'

  selectedOptionDidChange: (->
    if @get('selectedOption')
      @set 'selectedTemplate', null
  ).observes 'selectedOption'

  templatesDidChange: (->
    return unless App.get('templates.length') > 0
    unless @get 'selectedTemplate'
      @set 'selectedTemplate', App.get('templates.firstObject')
  ).observes 'App.templates.@each'

  selectedTemplateDidChange: (->
    if selectedTemplate = @get('selectedTemplate')
      @set 'selectedOption', null
      $('input[name=option]').removeAttr('checked')
      @set 'controller.privileged', selectedTemplate.get('privileged')
      unless @get('controller.nameFieldEdited')
        @set 'controller.name', selectedTemplate.get('name')
      if App.get('onboardingScreen') == 'pick-container'
        App.trigger 'next-onboarding'
  ).observes 'selectedTemplate'

  hostsDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      slug = @get('controller.preselectHost')
      if slug && !@get('controller.selectedHostSlug')
        @set 'controller.selectedHostSlug', slug
      @$('select[name=host]')?.change()
      @$('select[name=containers-new-option-host-image]')?.change()
      @$('select[name=containers-config-selected-volume]')?.change()
  ).observes 'App.hosts.@each'

  hostImagesDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('select[name=containers-new-option-host-image]')?.change()
  ).observes 'controller.selectedHost.images.@each'

  didInsertElement: ->
    @_super()

    # if a new app creation is already in progress,
    # directly go to that screen.
    if @get('controller.saving')
      @set 'currentScreenIndex', screens.indexOf('progress')

    # warn user not to close the browser tab while pulling image
    $(window).bind 'beforeunload.new-container', =>
      if @get('controller.pullingImage')
        unloadMessage

    @$('input[name=option]').on('change.containers-new', (ev) =>
      if $(ev.target).is(':checked')
        @set 'selectedOption', ev.target.value
    ).change()

    @$('.option-input').on 'click.containers-new', (ev) =>
      Em.run.next this, ->
        @$("##{$(ev.target).attr('name')}").click()

    lastStatus = ""
    App.on "progressReceived", (ev) =>
      if ev.status != lastStatus && lastStatus != ""
        @get('terminal.term')?.writeln("")

      if ev.progress
        if ev.status == lastStatus
          @get('terminal.term')?.write("\r\x1b[K")
          @get('terminal.term')?.write(ev.progress)
        else
          @get('terminal.term')?.write(ev.progress)
      else if ev.status && ev.status != lastStatus
        @get('terminal.term')?.write(ev.status)
      lastStatus = ev.status

    Em.run.scheduleOnce 'afterRender', this, ->
      @injectTerminalIframe()
      Em.run.later(this, ->
        @$('[autofocus]')?.focus()
      , 200)

  willDestroyElement: ->
    @_super()
    @$('.option-input').off('click.containers-new')
    @$('input[name=option]').off('change.containers-new')

    App.off "progressReceived"
    $(window).unbind 'beforeunload.new-container'

  actions:
    create: ->
      @get('terminal')?.clear()
      @send 'goNext'

      # If user has no existing hosts, we need to create one first.
      controller = @get('controller')
      if App.get('noHost')
        controller.send('createWorkspaceFirst', @get('image'))
      else
        controller.send('create', @get('image'))

    goNext: ->
      i = @get 'currentScreenIndex'
      @set 'currentScreenIndex', Math.min(++i, 2)

    goBack: ->
      i = @get 'currentScreenIndex'
      @set 'currentScreenIndex', Math.max(--i, 0)

    goNextOrCreate: ->
      if App.get 'user.advanced'
        @send 'goNext'
      else
        @set 'currentScreenIndex', 1 # skip the config screen
        @send 'create'

    closeEnvVar: ->
      ui.hideModal('#env-vars-dialog')

    toggleLog: ->
      @toggleProperty 'showLog'

    showHelp: ->
      App.set 'onboardingScreen', 'pick-container'

App.ContainerNameTextField = Em.TextField.extend
  click: ->
    unless @get('targetObject.nameFieldEdited')
      @$().select()

  keyPress: ->
    @set 'targetObject.nameFieldEdited', true

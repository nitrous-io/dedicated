#= require ../mixins/inject-terminal-iframe-mixin

optionHostImage = 'host-image'
optionBaseImage = 'base-image'
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

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

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
      else
        @get 'customImage'
  ).property 'selectedOption', 'selectedHostImage', 'customImage', 'controller.hostImagesEmpty'

  firstScreenButtonLabel: (->
    if App.get('user.advanced')
      'Next'
    else
      'Create'
  ).property 'App.user.advanced'

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug') && @get('controller.selectedHostSlug') && (@get('image') || @get('selectedTemplate')))
  ).property 'controller.name', 'controller.slug', 'controller.selectedHostSlug', 'image', 'selectedTemplate'

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.image.progress.progressDetail.start')

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
    if @get 'selectedTemplate'
      @set 'selectedOption', null
      $('input[name=option]').removeAttr('checked')
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
    App.slug.slugify(@$('#containers-new-name'), @$('#containers-new-slug'))

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
      @injectTerminalIframe =>
        @$('#containers-new-name').focus()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#containers-new-name'))
    @$('.option-input').off('click.containers-new')
    @$('input[name=option]').off('change.containers-new')

    App.off "progressReceived"
    $(window).unbind 'beforeunload.new-container'

  actions:
    create: ->
      @get('terminal')?.clear()
      @send 'goNext'
      @get('controller').send('create', @get('image'), @get('selectedOption') == optionBaseImage)

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

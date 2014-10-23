optionHostImage = 'host-image'
optionBaseImage = 'base-image'
screens = ['image', 'config', 'progress']

App.ContainersNewView = Lib.ContainerConfigView.extend
  elementId: 'containers-new-view'
  classNames: ['ui', 'card']

  selectedOption: null
  selectedHostImage: null
  selectedBaseImage: null
  customImage: null
  terminal: null

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

  currentScreenIndex: 0
  showImageScreen: Em.computed.equal('currentScreenIndex', screens.indexOf('image'))
  showConfigScreen: Em.computed.equal('currentScreenIndex', screens.indexOf('config'))
  showProgressScreen: Em.computed.equal('currentScreenIndex', screens.indexOf('progress'))

  image: (->
    option = @get 'selectedOption'
    switch option
      when optionHostImage
        unless @get 'controller.hostImagesEmpty'
          @get 'selectedHostImage'
      when optionBaseImage
        @get 'selectedBaseImage'
      else
        @get 'customImage'
  ).property 'selectedOption', 'selectedHostImage', 'selectedBaseImage', 'customImage', 'controller.hostImagesEmpty'

  baseImageOptionDisabled: (->
    !App.get('baseImages') || @get('controller.saving')
  ).property 'App.baseImages', 'controller.saving'
  hostImageOptionDisabled: Em.computed.or('controller.hostImagesEmpty', 'controller.saving')

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug') && @get('controller.selectedHostSlug') && @get('image'))
  ).property 'controller.name', 'controller.slug', 'controller.selectedHostSlug', 'image'

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.image.progress.progressDetail.start')

  preselectHostDidChange: (->
    if slug = @get('controller.preselectHost')
      @set 'controller.selectedHostSlug', slug
  ).observes 'controller.preselectHost'

  baseImagesDidChange: (->
    return unless App.get('baseImages.length') > 0
    unless @get 'selectedBaseImage'
      @set 'selectedBaseImage', App.get('baseImages.firstObject.repo')
  ).observes 'App.baseImages.@each'

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

    if App.get('baseImages.length') > 0 && !@get('selectedBaseImage')
      @set 'selectedBaseImage', App.get('baseImages.firstObject.repo')

    @$('input[name=option]').on('change.containers-new', (ev) =>
      if $(ev.target).is(':checked')
        @set 'selectedOption', ev.target.value
    ).change()

    @$('.option-input').on 'click.containers-new', (ev) =>
      Em.run.next this, ->
        @$("##{$(ev.target).attr('name')}").click()

    lastMsg = ""
    App.on "progressReceived", (ev) =>
      if ev.status && ev.status != lastMsg
        lastMsg = ev.status
        @get('terminal.term')?.writeln(ev.status)

    Em.run.scheduleOnce 'afterRender', this, ->
      @injectTerminalIframe()  
        
  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#containers-new-name'))
    @$('.option-input').off('click.containers-new')
    @$('input[name=option]').off('change.containers-new')

    App.off "progressReceived"

  injectTerminalIframe: ->
    Em.run.scheduleOnce 'afterRender', this, ->
      controller = @get 'controller'
      $iframe = $('<iframe>')
      @$('.terminal-screen').append($iframe)
      iframeWindow = $iframe[0].contentWindow
      iframeDocument = iframeWindow.document

      # use pure js dom api because jquery fails to use iframe's window/document obj
      linkEl = iframeDocument.createElement('link')
      linkEl.setAttribute('rel', 'stylesheet')
      linkEl.setAttribute('href', location.origin + App.get('terminalCssPath'))
      iframeDocument.head.appendChild(linkEl)

      scriptEl = iframeDocument.createElement('script')
      scriptEl.setAttribute('src', location.origin + App.get('terminalJsPath'))
      
      scriptEl.onload = =>
        scriptEl.onload = null
        terminal = iframeWindow.NitrousTerminal
        terminal.init()
        terminal.focus()
        @set 'terminal', terminal
        
      iframeDocument.body.appendChild(scriptEl)

  actions:
    create: ->
      @get('terminal')?.clear()
      @send 'goNext'
      @get('controller').send 'create', @get('image')

    goNext: ->
      i = @get 'currentScreenIndex'
      @set 'currentScreenIndex', Math.min(++i, 2)

    goBack: ->
      i = @get 'currentScreenIndex'
      @set 'currentScreenIndex', Math.max(--i, 0)

    closeEnvVars: ->
      ui.hideModal('#env-vars-dialog')

    create: ->
      @get('controller').send('create', @get('image'), @get('selectedOption') == optionBaseImage)

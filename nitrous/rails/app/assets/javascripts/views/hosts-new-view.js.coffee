App.HostsNewView = Em.View.extend App.InjectTerminalIframeMixin,
  elementId: 'hosts-new-view'
  classNames: ['ui', 'card']

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

  showLog: false

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug') && @get('controller.selectedRegionSlug') && @get('controller.selectedPlan')) || @get('controller.saving')
  ).property 'controller.name', 'controller.slug', 'controller.selectedRegionSlug', 'controller.selectedPlan', 'controller.saving'

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.host.progress.progressDetail.start')

  preselectRegion: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('#hosts-new-region').change()
  ).observes 'App.regions.@each'

  didInsertElement: ->
    @_super()
    controller = @get 'controller'

    @$().on 'change.hosts-new', 'input[name=region]', =>
      controller.set 'selectedRegionSlug', @$('input[name=region]').filter(':checked').val()

    @preselectRegion()

    App.slug.slugify(@$('#hosts-new-name'), @$('#hosts-new-slug'))

    lastStatus = ""
    App.on "progressReceived", (ev) =>
      if ev.stream
        @get('terminal.term')?.writeln(ev.stream)

      if ev.status != lastStatus && lastStatus != ""
        @get('terminal.term')?.writeln("")

      if ev.progress
        if ev.status == lastStatus
          @get('terminal.term')?.write("\r\x1b[K") # escape sequence to show progress
          @get('terminal.term')?.write(ev.progress)
        else
          @get('terminal.term')?.write(ev.progress)
      else if ev.status && ev.status != lastStatus
        @get('terminal.term')?.write(ev.status)
      lastStatus = ev.status

    Em.run.scheduleOnce 'afterRender', this, ->
      @injectTerminalIframe()

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#hosts-new-name'))
    @$().off 'change.hosts-new'

  actions:
    create: ->
      @get('terminal')?.clear()
      @get('controller').send 'create'

    toggleLog: ->
      @toggleProperty 'showLog'

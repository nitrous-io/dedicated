App.HostsNewView = Em.View.extend
  elementId: 'hosts-new-view'
  classNames: ['ui', 'card']

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')
  slugFieldInvalid: Em.computed.equal('controller.invalidField', 'slug')

  terminal: null

  selectedTemplate: null

  showLog: false

  createButtonDisabled: (->
    !(@get('controller.name') && @get('controller.slug') && @get('controller.selectedRegionSlug') && @get('controller.selectedPlanSlug')) || @get('controller.saving')
  ).property 'controller.name', 'controller.slug', 'controller.selectedRegionSlug', 'controller.selectedPlanSlug', 'controller.saving'

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.host.progress.progressDetail.start')

  preselectRegion: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('#hosts-new-region').change()
  ).observes 'App.regions.@each'

  preselectPlan: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('#hosts-new-plan').change()
  ).observes 'App.plans.@each'

  templatesDidChange: (->
    return unless App.get('templates.length') > 0
    unless @get 'selectedTemplate'
      @set 'selectedTemplate', App.get('templates.firstObject.slug')
  ).observes 'App.templates.@each'

  selectedTemplateDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      Em.run.next this, ->
        @$('.description').css 'height', @$('.description p').height()
  ).observes 'selectedTemplate'

  didInsertElement: ->
    @_super()
    controller = @get 'controller'

    @templatesDidChange()

    @$().on 'change.hosts-new', 'input[name=region]', =>
      controller.set 'selectedRegionSlug', @$('input[name=region]').filter(':checked').val()

    @$().on 'change.hosts-new', 'input[name=size]', =>
      controller.set 'selectedPlanSlug', @$('input[name=size]').filter(':checked').val()

    @preselectRegion()
    @preselectPlan()

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
      App.injectTerminalIframe this

  willDestroyElement: ->
    @_super()
    App.slug.unslugify(@$('#hosts-new-name'))
    @$().off 'change.hosts-new'

  actions:
    create: ->
      @get('terminal')?.clear()
      @get('controller').send 'create', @get('selectedTemplate.slug')

    toggleLog: ->
      @toggleProperty 'showLog'

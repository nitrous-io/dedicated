#= require ../mixins/inject-terminal-iframe-mixin

App.HostsNewView = Em.View.extend App.InjectTerminalIframeMixin,
  elementId: 'hosts-new-view'
  classNames: ['ui', 'card']

  nameFieldInvalid: Em.computed.equal('controller.invalidField', 'name')

  showLog: false

  submitButtonDisabled: (->
    !@get('controller.selectedRegion') || !@get('controller.selectedPlan') || @get('controller.saving')
  ).property 'controller.selectedRegion', 'controller.selectedPlan', 'controller.saving'

  submitButtonLabel: (->
    if App.get('newUser')
      'Next'
    else
      'Create'
  ).property 'App.newUser'

  hideProgress: Em.computed.not('controller.saving')
  indeterminateProgress: Em.computed.none('controller.host.progress.progressDetail.start')

  didInsertElement: ->
    @_super()

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
      Em.run.later(this, ->
        @$('[autofocus]').focus()
      , 200)

  willDestroyElement: ->
    @_super()

  selectRegionDidChange: (->
    if App.get('onboardingScreen') == 'select-region'
      App.trigger 'next-onboarding'
  ).observes 'controller.selectedRegion'

  actions:
    create: ->
      @get('terminal')?.clear()
      @get('controller').send 'create'

    toggleLog: ->
      @toggleProperty 'showLog'

    showHelp: ->
      App.set 'onboardingScreen', 'select-region'

App.HostNameTextField = Em.TextField.extend
  click: ->
    unless @get('targetObject.nameFieldEdited')
      @$().select()

  keyPress: ->
    @set 'targetObject.nameFieldEdited', true

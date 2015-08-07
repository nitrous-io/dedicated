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
    if @get('controller.hasNextStep')
      'Next'
    else
      'Create'
  ).property 'controller.hasNextStep'

  hideProgress: (->
    if @get('controller.selectedPlan.isFreePlan')
      return true
    !@get('controller.saving')
  ).property 'controller.saving', 'controller.selectedPlan.isFreePlan'

  country: (->
    countries = @get('controller.countryDict')
    countries[App.get('user.phoneCountryCode')]
  ).property 'controller.countryDict', 'App.user.phoneCountryCode'

  freePlanUnavailable: (->
    return true if @get 'controller.freePlanNotAllowed'
    App.get('billingProfile.isNoCardState') && @get('controller.freePlans.length') == 0
  ).property 'controller.freePlanNotAllowed', 'billingProfile.isNoCardState', 'controller.freePlans.length'

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
      @$('select[name=hosts-new-owner]').on 'change.hosts-new', (e) =>
        selected = $(e.target).val()
        selected = null if selected == '(user)'
        @set 'controller.selectedOrgSlug', selected

      @injectTerminalIframe()
      Em.run.later(this, ->
        @$('[autofocus]').focus()
      , 200)

  willDestroyElement: ->
    @_super()
    @$('select[name=hosts-new-owner]').off 'change.hosts-new'

  selectRegionDidChange: (->
    if App.get('onboardingScreen') == 'select-region'
      App.trigger 'next-onboarding'
  ).observes 'controller.selectedRegion'

  actions:
    create: ->
      # If user already has a valid card on file, show a confirmation dialog
      # because they will be billed extra for creating a new workspace.
      if @get('controller.selectedPlan.isPaidPlan') && App.get('billingProfile.hasCard')
        ui.showModal('#confirm-new-subscription-modal-dialog')
        return

      @send('createWithConfirmation')

    createWithConfirmation: ->
      ui.hideModal('#confirm-new-subscription-modal-dialog')
      @get('terminal')?.clear()
      @get('controller').send 'create'

    cancelConfirmation: ->
      ui.hideModal('#confirm-new-subscription-modal-dialog')

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

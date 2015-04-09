states = [
    "welcome",
    "pick-container",
    "select-container-name",
    "waiting-host", # we wait for the controller to give the go ahead
    "select-region",
    "select-plan",
    "waiting-billing", # wait
    "cardholders-name",
    "waiting-finish"
  ]

App.OnboardingController = Em.ObjectController.extend
  screenBinding: 'App.onboardingScreen'

  startWelcome: (->
    if App.get 'onboardingReady'
      @set '_screen', 'welcome'
  ).property 'App.onboardingReady'

  nextScreen: (->
    curIndex = states.indexOf(@get 'screen')
    (states[curIndex + 1] || null)
  ).property 'screen'

  clear: ->
    @_super.apply this, arguments
    @set 'screen', 'welcome'

  actions:
    dismissOnboarding: ->
      App.set 'onboardingReady', false

    nextScreen: ->
      @set 'screen', @get 'nextScreen'


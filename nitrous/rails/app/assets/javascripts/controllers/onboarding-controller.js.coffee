screens = [
    "welcome",
    "pick-container",
    "select-container-name",
    "waiting-host", # we wait for the controller to give the go ahead
    "select-region",
    "select-plan",
    "waiting-billing", # wait
    "cardholders-name",
    "waiting-create",
    "thank-you",
    "waiting-finish"
  ]

App.OnboardingController = Em.ObjectController.extend
  screenBinding: 'App.onboardingScreen'

  bindEvents: (->
    App.on 'next-onboarding', ()=>
      @send 'nextScreen'
    App.on 'handle-transition', (url)=>
      @send 'handleTransition', url
  ).on 'init'

  startWelcome: (->
    if App.get 'onboardingReady'
      @set 'screen', 'welcome'
  ).observes 'App.onboardingReady'

  nextScreen: (->
    curIndex = screens.indexOf(@get 'screen')
    (screens[curIndex + 1] || null)
  ).property 'screen'

  nextWaiting: (->
    curIndex = screens.indexOf(@get 'screen')
    remaining = screens.slice curIndex
    screen = null
    remaining.some (el, indx)=>
      if el.match(/^waiting-/)
        screen = el
        return true
      return false
    return screen
  ).property 'screen'

  clear: ->
    @_super.apply this, arguments
    @set 'screen', 'welcome'

  actions:
    dismissOnboarding: ->
      @set 'screen', null
      App.set 'onboardingReady', false

    nextScreen: ->
      @set 'screen', @get 'nextScreen'

    handleTransition: (url)->
      return unless @get 'screen'

      screen = null
      if url.indexOf('containers/new') != -1
        return if (@screen == 'welcome') || (url.indexOf('runCreate') != -1)
        screen = 'pick-container'
      if url.indexOf('workspaces/new') != -1
        return if (@screen == 'thank-you')
        screen = 'select-region'
      if url.indexOf('workspaces/add-card') != -1
        screen = 'cardholders-name'
      if url.indexOf('add-card') != -1
        screen = 'cardholders-name'

      curIndex = screens.indexOf(@get 'screen')
      newIndex = screens.indexOf(screen)
      if newIndex <= curIndex
        screen = 'go-back'
      @set 'screen', screen



App.OnboardingView = Em.View.extend
  elementId: 'onboarding-view'

  screenBinding: 'controller.screen'

  showGutter: (->
    @get('screen') != 'welcome'
  ).property 'screen'

  didInsertElement: ->
    @_super()

  controlWelcome: (->
    if @get('screen') == 'welcome'
      ui.showModal('#onboarding-welcome')
    else
      ui.hideModal('#onboarding-welcome')
  ).observes 'screen'

  controlThankyou: (->
    if @get('screen') == 'thank-you'
      ui.showModal('#thankyou')
    else
      ui.hideModal('#thankyou')
  ).observes 'screen'
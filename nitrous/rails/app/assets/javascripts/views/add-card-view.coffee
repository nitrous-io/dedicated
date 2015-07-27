App.AddCardView = Em.View.extend
  elementId: 'add-card-view'
  classNames: ['ui', 'card']

  numberFieldInvalid: Em.computed.equal('controller.invalidField', 'credit card number')
  expirationFieldInvalid: Em.computed.equal('controller.invalidField', 'expiration')
  cvcFieldInvalid: Em.computed.equal('controller.invalidField', 'cvc')
  zipFieldInvalid: Em.computed.equal('controller.invalidField', 'zip')

  didInsertElement: ->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('#card-number').payment 'formatCardNumber'
      @$('#card-expiration').payment 'formatCardExpiry'
      @$('#card-cvc').payment 'formatCardCVC'
      @$('[autofocus]').focus()

  saveButtonDisabled: (->
    !(@get('controller.name') && @get('controller.number') && @get('controller.expiration') && @get('controller.cvc')) || @get('controller.saving')
  ).property 'controller.name', 'controller.number', 'controller.expiration', 'controller.cvc', 'controller.saving'

  showCardIcon: (->
    backgroundImageURL = "none"
    if cardType = $.payment.cardType @get('controller.number')
      backgroundImageURL = "url(#{location.protocol}//nitrous-image-icons.s3.amazonaws.com/cards/#{cardType}.png)"
    $('#card-number').css 'background-image', backgroundImageURL
  ).observes 'controller.number'

  actions:
    showHelp: ->
      App.set 'onboardingScreen', 'cardholders-name'

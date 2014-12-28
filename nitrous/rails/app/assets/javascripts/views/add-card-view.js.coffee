App.AddCardView = Em.View.extend
  elementId: 'add-card-view'
  classNames: ['ui', 'card']

  numberFieldInvalid: Em.computed.equal('controller.invalidField', 'credit card number')
  expirationFieldInvalid: Em.computed.equal('controller.invalidField', 'expiration')
  cvcFieldInvalid: Em.computed.equal('controller.invalidField', 'cvc')
  zipFieldInvalid: Em.computed.equal('controller.invalidField', 'zip')

  didInsertElement: ->
    $('#card-number').payment 'formatCardNumber'
    $('#card-expiration').payment 'formatCardExpiry'
    $('#card-cvc').payment 'formatCardCVC'

  saveButtonDisabled: (->
    !(@get('controller.name') && @get('controller.number') && @get('controller.expiration') && @get('controller.cvc')) || @get('controller.saving')
  ).property 'controller.name', 'controller.number', 'controller.expiration', 'controller.cvc', 'controller.saving'

App.BillingView = Em.View.extend
  elementId: 'billing-view'
  classNames: ['ui', 'card']

  numberFieldInvalid: Em.computed.equal('controller.invalidField', 'number')
  expirationFieldInvalid: Em.computed.equal('controller.invalidField', 'expiration')
  cvcFieldInvalid: Em.computed.equal('controller.invalidField', 'cvc')

  saveButtonDisabled: (->
    !(@get('controller.name') && @get('controller.number') && @get('controller.expiration') && @get('controller.cvc')) || @get('controller.saving')
  ).property 'controller.name', 'controller.number', 'controller.expiration', 'controller.cvc', 'controller.saving'

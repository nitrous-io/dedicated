App.AccountView = Em.View.extend
  elementId: 'account-view'
  classNames: ['ui', 'card']

  emailFieldInvalid:    Em.computed.equal('controller.detailsInvalidField', 'email')
  passwordFieldInvalid: Em.computed.equal('controller.detailsInvalidField', 'password')

  showConfirmPassword: (->
    @get('controller.password') && @get('controller.password').length > 0
  ).property 'controller.password'

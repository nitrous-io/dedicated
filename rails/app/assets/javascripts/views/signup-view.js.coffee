App.SignupView = Em.View.extend
  elementId: 'signup-view'

  nameFieldInvalid:     Em.computed.equal('controller.invalidField', 'name')
  usernameFieldInvalid: Em.computed.equal('controller.invalidField', 'username')
  emailFieldInvalid:    Em.computed.equal('controller.invalidField', 'email')
  passwordFieldInvalid: Em.computed.equal('controller.invalidField', 'password')

  disableSubmitButton: (->
    !@get('controller.user.name') || !@get('controller.user.username') || !@get('controller.user.email') || !@get('controller.user.password') || !@get('controller.confirmPassword') || @get('controller.signingUp')
  ).property 'controller.user.name', 'controller.user.username', 'controller.user.email', 'controller.user.password', 'controller.confirmPassword', 'controller.signingUp'

  focusInvalidField: (->
    if field = @get 'controller.invalidField'
      @$("#signup-#{field}").focus()
  ).observes('controller.invalidField')

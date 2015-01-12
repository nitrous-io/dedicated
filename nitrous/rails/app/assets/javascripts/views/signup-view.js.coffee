#= require mailcheck

App.SignupView = Em.View.extend
  elementId: 'signup-view'

  codeFieldInvalid:     Em.computed.equal('controller.invalidField', 'code')
  usernameFieldInvalid: Em.computed.equal('controller.invalidField', 'username')
  emailFieldInvalid:    Em.computed.equal('controller.invalidField', 'email')
  passwordFieldInvalid: Em.computed.equal('controller.invalidField', 'password')

  disableSubmitButton: (->
    !@get('controller.user.username') || !@get('controller.user.email') || !@get('controller.user.password') || @get('controller.signingUp')
  ).property 'controller.user.username', 'controller.user.email', 'controller.user.password', 'controller.signingUp'

  focusInvalidField: (->
    if field = @get 'controller.invalidField'
      @$("#signup-#{field}").focus()
  ).observes('controller.invalidField')

  checkEmail: (->
    Mailcheck.run(
      email: @get('controller.user.email'),
      suggested: (s)=>
        @set('controller.suggestedEmail', s.full)
      empty: (s)=>
        @set('controller.suggestedEmail', null)
    )
  ).observes('controller.user.email')

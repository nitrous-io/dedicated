App.ForgotPasswordView = Em.View.extend
  elementId: 'forgot-password-view'

  disableSubmitButton: (->
    !@get('controller.login') || @get('controller.saving')
  ).property 'controller.login', 'controller.saving'

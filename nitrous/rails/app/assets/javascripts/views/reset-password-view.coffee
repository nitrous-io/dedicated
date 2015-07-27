App.ResetPasswordView = Em.View.extend
  elementId: 'reset-password-view'

  disableSubmitButton: (->
    !@get('controller.password') || !@get('controller.confirmPassword') || @get('controller.saving')
  ).property 'controller.login', 'controller.confirmPassword', 'controller.saving'

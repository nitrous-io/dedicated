App.AddPhoneView = Em.View.extend
  elementId: 'add-phone-view'
  classNames: ['ui', 'card']

  didInsertElement: ->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('[autofocus]').focus()

  phoneFormDisabled: (->
    @get('controller.saving') || @get('controller.otpSent')
  ).property 'controller.saving', 'controller.otpSent'

  resendOTPDisabled: (->
    @get('controller.saving') || @get('controller.verifying')
  ).property 'controller.saving', 'controller.verifying'

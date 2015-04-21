App.AddPhoneView = Em.View.extend
  elementId: 'add-phone-view'
  classNames: ['ui', 'card']

  didInsertElement: ->
    this.$('option[value="disabled"]').attr('disabled', true)
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('[autofocus]').focus()

  phoneFormDisabled: (->
    @get('controller.saving') || @get('controller.otpSent')
  ).property 'controller.saving', 'controller.otpSent'

  resendOTPDisabled: (->
    @get('controller.saving') || @get('controller.verifying')
  ).property 'controller.saving', 'controller.verifying'

App.AddPhoneController = Em.ObjectController.extend
  phone: null
  phoneCountryCode: null
  otp: null

  otpSent: false
  saving: false
  verifying: false

  invalidOTP: false

  countryList: null

  clear: ->
    @_super.apply this, arguments
    @setProperties
      phone: null
      phoneCountryCode: null
      otp: null

      otpSent: false
      saving: false
      verifying: false

      invalidOTP: false

  fullPhone: (->
    "+#{@get('phoneCountryCode')}-#{@get('phone')}"
  ).property 'phone', 'phoneCountryCode'

  actions:
    savePhone: ->
      @set 'saving', true

      user = App.get('user')
      user.updatePhone(@get('phone'), @get('phoneCountryCode')).then (resp) =>
        @set 'otpSent', true
        @set 'saving', false
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Could not use that phone number, please try another number.')
        @set 'saving', false

    verifyOTP: ->
      @set 'verifying', true

      user = App.get('user')
      user.verifyPhone(@get('otp')).then (resp) =>
        @set 'verifying', false
        if !resp.verified
          App.postError 'That code is incorrect.'
          @set 'invalidOTP', true
          return
        user.set('phoneAuthed', true)
        @transitionToRoute 'hosts.new'
      , (jqxhr) =>
        @set 'verifying', false
        App.postError(jqxhr.responseJSON, 'That code is incorrect.')
        @set 'invalidOTP', true

    resendOTP: ->
      @set 'saving', true

      App.get('user').resendPhoneCode().then (resp) =>
        @set 'saving', false
        App.snackbar.post "A new verification code has been sent to #{@get 'fullPhone'}"
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'There was an error sending an SMS to your phone, please try another number.')
        @set 'saving', false


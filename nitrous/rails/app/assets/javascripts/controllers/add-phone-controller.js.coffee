App.AddPhoneController = Em.ObjectController.extend
  phone: null
  phoneCountryCode: null
  otp: null

  otpSent: false
  otpAttempts: 0
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
      phone = @get('phone')
      phoneCountryCode = @get('phoneCountryCode')

      # Strip country code from phone number if present.
      if phone.indexOf("+#{phoneCountryCode}") == 0
        phone = phone.replace("+#{phoneCountryCode}", '')
      # Preserve only numbers in phone number.
      phone = phone.replace(/[^0-9]/g, '')
      @set('phone', phone)

      App.get('user').updatePhone(phone, phoneCountryCode).then (resp) =>
        Mp.timestamp Mp.PropertyPhoneNumberSaved
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
        Mp.timestamp Mp.PropertyPhoneNumberVerified
        p = {}
        p[Mp.PropertyPhoneNumber] = "+#{@get('phoneCountryCode')}-#{@get('phone')}"
        Mp.updateUser p
        App.refreshBillingProfile()

        @transitionToRoute 'home'
      , (jqxhr) =>
        @set 'verifying', false
        App.postError(jqxhr.responseJSON, 'That code is incorrect.')
        @set 'invalidOTP', true

    resendOTP: ->
      if @get('otpAttempts') >= 3
        App.postError 'Too many attempts.'
        return
      @incrementProperty 'otpAttempts'
      @set 'saving', true

      App.get('user').resendPhoneCode().then (resp) =>
        Mp.increment Mp.PropertyPhoneNumberSMSCount
        @set 'saving', false
        App.snackbar.post "A new verification code has been sent to #{@get 'fullPhone'}"
      , (jqxhr) =>
        Mp.increment Mp.PropertyPhoneNumberSMSCount
        App.postError(jqxhr.responseJSON, 'There was an error sending an SMS to your phone, please try another number.')
        @set 'saving', false

    editPhone: ->
      @set 'otpSent', false

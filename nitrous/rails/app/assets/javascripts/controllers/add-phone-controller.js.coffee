App.AddPhoneController = Em.ObjectController.extend
  phone: null
  phoneCountryCode: null
  otp: null

  otpSent: false
  saving: false
  verifying: false

  invalidOTP: false

  whitelist: null

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
        App.postError(jqxhr.responseJSON, 'Could not use that phone number, please try another phone.')
        @set 'saving', false

    verifyOTP: ->
      @set 'verifying', true

      # TODO when fail
      @set 'invalidOTP', true

      # TODO when success,
      # @set 'verifying', false
      # @transitionToRoute 'hosts.new'

    resendOTP: ->


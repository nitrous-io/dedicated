App.LoginController = Em.Controller.extend
  confirmationLapsed: false

  confirmationLapsedResendable: (->
    App.get('confirmationLapsed') && App.get('resendConfirmationAppToken') && App.get('resendConfirmationAppToken').length > 0
  ).property 'App.confirmationLapsed', 'App.resendConfirmationAppToken'

  login: null
  password: null

  error: null
  loggingIn: false

  doLogin: (login, password) ->
    App.set 'confirmationLapsed', false
    @set 'loggingIn', true
    @set 'error', null
    App.authenticate(login, password).then((token) =>
      App.setToken(token)
      Mp.track Mp.EventUserLoggedIn

      if coupon = App.get('pendingCoupon')
        @transitionToRoute 'coupon', queryParams: { code: coupon }
        return

      @transitionToRoute 'home.index'
    , (err) =>
      if err
        @set 'error', err
        App.postError(err.message)
    ).then =>
      @set 'loggingIn', false

  actions:
    login: ->
      @doLogin(@get('login'), @get('password'))

    resendConfirmationEmail: ->
      @set 'resendingConfirm', true
      @set 'resentConfirm', false

      Em.RSVP.Promise.cast(Em.$.ajax(
        type: 'POST'
        url: "/api/v0/user/resend_confirm?app_token=#{App.get('resendConfirmationAppToken')}"
        cache: false
        contentType: false
      )).then (json) =>
        @set 'resendingConfirm', false
        @set 'resentConfirm', true
        Mp.track Mp.EventUserResentConfirmationEmail
      , (jqxhr) =>
        @set 'resendingConfirm', false
        @set 'generalError', true

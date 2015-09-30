App.LoginController = Em.Controller.extend
  queryParams: ['token']
  token: null
  confirmationLapsed: false

  confirmationLapsedResendable: (->
    App.get('confirmationLapsed') && App.get('resendConfirmationAppToken') && App.get('resendConfirmationAppToken').length > 0
  ).property 'App.confirmationLapsed', 'App.resendConfirmationAppToken'

  login: null
  password: null

  error: null
  loggingIn: false

  tokenChanged: (->
    if @get('token')
      @afterLogin(@get('token'))
  ).observes('token')

  doLogin: (login, password) ->
    App.set 'confirmationLapsed', false
    @set 'loggingIn', true
    @set 'error', null
    App.authenticate(login, password).then((token) =>
      @afterLogin(token)
    , (err) =>
      if err
        @set 'error', err
        App.postError(err.message)
    ).then =>
      @set 'loggingIn', false

  doLoginWithGitHub: ->
    @set 'loggingInWithGitHub', true
    @set 'error', null

    Em.$.ajax(
      type: 'POST'
      url: '/api/v0/github/authorize_to_login'
      cache: false
    ).then (json) =>
      window.location.href = json.redirect_url
    , (err) =>
      if err
        @set 'error', err
        App.postError(err.message)

  afterLogin: (token) ->
    App.setToken(token)
    Mp.track Mp.EventUserLoggedIn

    if coupon = App.get('pendingCoupon')
      @transitionToRoute 'coupon', queryParams: { code: coupon }
      return

    @transitionToRoute 'home.index'

  actions:
    login: ->
      @doLogin(@get('login'), @get('password'))

    loginWithGitHub: ->
      @doLoginWithGitHub()

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

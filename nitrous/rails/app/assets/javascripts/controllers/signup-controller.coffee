App.SignupController = Em.Controller.extend App.PromoCodeInfoMixin,
  queryParams: [{ incomingEmail: 'email', promoCode: 'promo', inviteCode: 'invite_code', intent: 'intent', gitHubUserName: 'gitHubUserName', gitHubUserEmail: 'gitHubUserEmail' }]
  passibleChars: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOP1234567890"
  user: null
  signingUp: false
  error: null
  invalidField: null
  suggestedEmail: null
  promoCode: null
  inviteCode: null
  intent: null
  signingUpWithGitHub: false
  gitHubUserName: null
  gitHubUserEmail: null

  clear: ->
    user = App.User.create()
    @setProperties
      user: user
      signingUp: false
      error: null
      invalidField: null
    user.set('username', @get('gitHubUserName'))
    user.set('email', @get('gitHubUserEmail'))

  emailDisabled: (->
    return true if @get 'signingUp'
    @get('inviteCode') && @get('user.email')
  ).property 'inviteCode', 'user.email', 'signingUp'

  showSuggested: (->
    return false if @get('emailDisabled')
    @get 'suggestedEmail'
  ).property 'suggestedEmail', 'emailDisabled'

  handleIncomingEmail: ->
    return unless email = @get("incomingEmail")
    @get('user').setProperties
      email: email
      username: email.match(/^([^@]+)@/)?[1]
    App.User.adapter.ajax('POST', '/users/email_signup', email: email)

  signupWithGitHub: ->
    @set 'signingUpWithGitHub', true
    @set 'error', null

    Em.$.ajax(
      type: 'POST'
      url: '/api/v0/github/authorize_to_signup'
      cache: false
    ).then (json) =>
      window.location.href = json.redirect_url
    , (err) =>
      @set 'signingUpWithGitHub', false
      if err
        @set 'error', err
        App.postError(err.message)

  randomPassword: ->
    password = ""
    for i in [0..20]
      i = Math.floor(Math.random() * @passibleChars.length)
      password += @passibleChars.charAt(i)
    password

  actions:
    acceptSuggestedEmail: ->
      if @get('suggestedEmail')
        @set('user.email', @get('suggestedEmail'))

    signup: ->
      @set 'error', null
      @set 'invalidField', null
      @set 'signingUp', true
      user = @get 'user'

      if @get('gitHubUserName')
        user.set('password', @randomPassword())

      if distinctId = window.mixpanel?.get_distinct_id?()
        user.set('distinctId', distinctId)

      user.save(promo_code: @get('promoCode'), invite_code: @get('inviteCode'), github_username: @get('gitHubUserName')).then(=>
        if @get('inviteCode')
          App.set 'sharedContainerTip', true

        if intent = @get('intent')
          App.set 'intent', intent

        Mp.identify()
        App.__container__.lookup('controller:login').doLogin(user.get('username'), user.get('password'))
        user.set 'password', null
        Mp.track Mp.EventUserSignedUp
      , (jqxhr) =>
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'invalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to sign up!')
      ).then =>
        @set 'signingUp', false

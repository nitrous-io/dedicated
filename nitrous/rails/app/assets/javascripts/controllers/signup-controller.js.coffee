App.SignupController = Em.Controller.extend App.PromoCodeInfoMixin,
  queryParams: [{ incomingEmail: 'email', promoCode: 'promo', inviteCode: 'invite_code' }]
  user: null
  signingUp: false
  error: null
  invalidField: null
  suggestedEmail: null
  promoCode: null
  inviteCode: null

  clear: ->
    @setProperties
      user: App.User.create()
      signingUp: false
      error: null
      invalidField: null

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

  actions:
    acceptSuggestedEmail: ->
      if @get('suggestedEmail')
        @set('user.email', @get('suggestedEmail'))

    signup: ->
      @set 'error', null
      @set 'invalidField', null
      @set 'signingUp', true
      user = @get 'user'

      if distinctId = window.mixpanel?.get_distinct_id?()
        user.set('distinctId', distinctId)

      user.save(promo_code: @get('promoCode'), invite_code: @get('inviteCode')).then(=>
        if @get('inviteCode')
          App.set 'sharedContainerTip', true

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

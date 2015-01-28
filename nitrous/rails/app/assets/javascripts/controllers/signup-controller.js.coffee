App.SignupController = Em.Controller.extend
  queryParams: [{ incomingEmail: 'email', promoCode: 'promo' }]
  user: null
  signingUp: false
  error: null
  invalidField: null
  suggestedEmail: null
  promoCode: null

  clear: ->
    @setProperties
      user: App.User.create()
      signingUp: false
      error: null
      invalidField: null

  handleIncomingEmail: ->
    return unless email = @get("incomingEmail")
    @get('user').setProperties
      email: email
      username: email.match(/^([^@]+)@/)?[1]
    App.User.adapter.ajax('POST', '/users/email_signup', email: email)

  promoCodeInfo: (->
    promoCode = @get('promoCode') || 'Signup_Credit'
    info = Em.ObjectProxy.create()
    Em.$.ajax(
      type: 'GET',
      url: "/api/v0/promo_codes/#{promoCode}"
    ).done((data) =>
      if amount = data.credit_amount
        info.set 'content', (amount / 100).toFixed(2)
    )
    info
  ).property 'promoCode'

  actions:
    acceptSuggestedEmail: ->
      if @get('suggestedEmail')
        @set('user.email', @get('suggestedEmail'))

    signup: ->
      @set 'error', null
      @set 'invalidField', null
      @set 'signingUp', true
      user = @get 'user'
      user.save(promo_code: @get('promoCode')).then(=>
        App.__container__.lookup('controller:login').doLogin(user.get('username'), user.get('password'))
        user.set 'password', null
      , (jqxhr) =>
        console.log jqxhr
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'invalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to sign up!')
      ).then =>
        @set 'signingUp', false

App.SignupController = Em.Controller.extend
  queryParams: [{ incomingEmail: 'email' }]
  code: null
  user: null
  signingUp: false
  error: null
  invalidField: null
  suggestedEmail: null

  clear: ->
    @setProperties
      code: null
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

  actions:
    acceptSuggestedEmail: ->
      if @get('suggestedEmail')
        @set('user.email', @get('suggestedEmail'))

    signup: ->
      @set 'error', null
      @set 'invalidField', null
      @set 'signingUp', true
      code = @get 'code'
      user = @get 'user'
      user.save(code: code).then(=>
        App.__container__.lookup('controller:login').doLogin(user.get('username'), user.get('password'))
        user.set 'password', null
      , (jqxhr) =>
        console.log jqxhr
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'invalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to sign up!')
      ).then =>
        @set 'signingUp', false

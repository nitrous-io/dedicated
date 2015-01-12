App.SignupController = Em.Controller.extend
  queryParams: ['incomingEmail']
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

  submitEmail: ->
    if @get("incomingEmail") && @get("incomingEmail").length > 0
      @get('user').set('email', @get('incomingEmail'))
      data =
        email: @get('incomingEmail')
      App.User.adapter.ajax('POST', '/users/email_signup', data)

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

App.SignupController = Em.Controller.extend
  code: null
  user: null
  confirmPassword: null
  signingUp: false
  error: null
  invalidField: null

  clear: ->
    @setProperties
      code: null
      user: App.User.create()
      confirmPassword: null
      signingUp: false
      error: null
      invalidField: null

  actions:
    signup: ->
      @set 'error', null
      @set 'invalidField', null

      if @get('user.password') != @get('confirmPassword')
        App.snackbar.post("Password doesn't match the confirmation.")
        return

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

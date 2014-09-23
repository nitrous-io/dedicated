App.SignupController = Em.Controller.extend
  user: null
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
      user = @get 'user'
      user.save().then(=>
        App.__container__.lookup('controller:login').doLogin(user.get('username'), user.get('password'))
        user.set 'password', null
      , (jqxhr) =>
        if errMsg = jqxhr.responseJSON?.error_description
          @set 'invalidField', errMsg.match(/^\w+/)[0]
        App.postError(jqxhr.responseJSON, 'Failed to sign up!')
      ).then =>
        @set 'signingUp', false

App.ResetPasswordController = Em.Controller.extend
  queryParams: ['token']
  password: null
  confirmPassword: null

  saving: false

  actions:
    resetPassword: ->
      if @get('password') != @get('confirmPassword')
        App.snackbar.post("Password doesn't match the confirmation.")
        return

      @set 'saving', true
      data =
        password: @get 'password'
        reset_token: @get 'token'

      App.User.adapter.ajax('POST', '/reset_password', data).then =>
        App.snackbar.post("Your password has been changed. Login with the new password.")
        @set 'saving', false
        @transitionToRoute '/'
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to connect!")
        @set 'saving', false
      false

App.ForgotPasswordController = Em.Controller.extend
  login: null
  saving: false

  actions:
    sendResetToken: ->
      @set 'saving', true
      data =
        login: @get 'login'

      App.User.adapter.ajax('POST', '/send_reset_password_token', data).then =>
        App.snackbar.post("You will receive an email to #{@get 'login'} with instructions about how to reset your password in a few minutes.", 10000)
        @set 'saving', false
        @transitionToRoute '/'
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to connect!')
        @set 'saving', false
      false

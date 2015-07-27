App.ForgotPasswordController = Em.Controller.extend
  login: null
  saving: false

  actions:
    sendResetToken: ->
      @set 'saving', true
      data =
        login: @get 'login'

      App.User.adapter.ajax('POST', '/send_reset_password_token', data).then =>
        login = @get 'login'
        prefix = "An email has been sent to #{login}"
        if login.indexOf('@') == -1
          # @ is not allowed for usernames, assume no @ = username
          prefix = "An email has been sent to the address associated with account '#{login}'"
        App.snackbar.post("#{prefix} with password reset instructions", 10000)
        @set 'saving', false
        @transitionToRoute '/'
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to connect!')
        @set 'saving', false
      false

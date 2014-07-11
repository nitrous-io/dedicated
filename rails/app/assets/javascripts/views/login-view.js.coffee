App.LoginView = Em.View.extend
  elementId: 'login-view'

  actions:
    shake: ->
      $loginButton = @$('#login-submit').attr('disabled', true).blur()
      $loginBox = @$('.login-box').addClass('shake')
      setTimeout(->
        $loginBox.removeClass('shake')
        $loginButton.attr('disabled', false)
      , 1000)

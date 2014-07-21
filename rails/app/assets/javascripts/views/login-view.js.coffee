App.LoginView = Em.View.extend
  elementId: 'login-view'

  actions:
    shake: ->
      $loginButton = @$('#login-submit-btn').attr('disabled', true).blur()
      $loginBox = @$('.ui.card').addClass('shake')
      setTimeout(->
        $loginBox.removeClass('shake')
        $loginButton.attr('disabled', false)
      , 1000)

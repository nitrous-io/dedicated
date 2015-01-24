App.LoginView = Em.View.extend
  elementId: 'login-view'

  disableSubmitButton: (->
    !@get('controller.login') || !@get('controller.password') || @get('controller.loggingIn')
  ).property 'controller.login', 'controller.password', 'controller.loggingIn'

  focusLoginField: (->
    if @get 'controller.error'
      @$('#login-login').select()
  ).observes 'controller.error'

  didInsertElement: ->
    @_super()
    Em.run.schedule 'afterRender', this, ->
      @$('#login-login').focus()

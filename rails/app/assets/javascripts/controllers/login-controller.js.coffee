App.LoginController = Em.Controller.extend
  login: null
  password: null
  error: null
  loggingIn: false

  actions:
    login: ->
      @set 'loggingIn', true
      @set 'error', null
      App.authenticate(@get('login'), @get('password')).then((token) =>
        App.set 'token', token
        @transitionToRoute 'home.index'
      , (err) =>
        if err
          @set 'error', err
          App.snackbar.post(err.message.capitalize())
      ).then =>
        @set 'loggingIn', false

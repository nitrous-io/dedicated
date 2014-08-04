App.LoginController = Em.Controller.extend
  login: null
  password: null
  error: null
  loggingIn: false

  actions:
    login: ->
      @set 'loggingIn', true
      @set 'error', null
      App.authenticate @get('login'), @get('password'), (err, token) =>
        @set 'loggingIn', false

        if err
          @set 'error', err
          App.snackbar.post(err.message.capitalize())
          return

        App.set 'token', token
        @transitionToRoute 'home.index'

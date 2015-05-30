#= require ./simple-model/rest-adapter

App.RESTAdapter = SM.RESTAdapter.extend
  namespace: '/api/v0'

  headers: (->
    Authorization: "Bearer #{App.get 'token'}"
  ).property 'App.token'

  ajax: ->
    ajax = @_super.apply(this, arguments)
    ajax.fail (jqxhr) =>
      if jqxhr.status == 401
        @logout()
      else if jqxhr.responseJSON && jqxhr.responseJSON.error == 'confirmation_lapsed'
        if App.get('token') && App.get('token').length > 0
          App.set 'resendConfirmationAppToken', App.get 'token'
          App.logout().then ->
            App.trigger 'confirmation-lapsed'
          , ->
            App.trigger 'confirmation-lapsed'
          App.postError "Your confirmation email has expired. You can resend one below."
          return

        if !App.get("confirmationLapsed")
          App.postError(jqxhr.responseJSON, "Confirmation token has expired")
    ajax

  logout: ->
    App.set 'token', null

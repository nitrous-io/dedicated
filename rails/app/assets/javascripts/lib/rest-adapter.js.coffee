#= require ./simple-model/rest-adapter

App.RESTAdapter = SM.RESTAdapter.extend
  namespace: '/api/v0'

  headers: (->
    Authorization: "Bearer #{App.get 'token'}"
  ).property 'App.token'

  ajax: ->
    ajax = @_super.apply(this, arguments)
    ajax.fail (jqxhr) ->
      if jqxhr.status == 401
        App.__container__.lookup('router:main').transitionTo 'logout'
        Em.run.next ->
          App.snackbar.post('Your access token is no longer valid, please log in again.')
    ajax

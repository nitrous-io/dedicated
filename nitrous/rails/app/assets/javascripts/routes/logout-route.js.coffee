App.LogoutRoute = Em.Route.extend
  beforeModel: (transition) ->
    @_super.apply this, arguments
    App.set 'token', null
    Em.run.next ->
      confirmationLapsed = transition?.queryParams.confirmationLapsed == true
      App.__container__.lookup('router:main').transitionTo 'login', queryParams: { confirmationLapsed: confirmationLapsed }

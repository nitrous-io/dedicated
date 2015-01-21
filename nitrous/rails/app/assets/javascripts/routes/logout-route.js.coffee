App.LogoutRoute = Em.Route.extend
  beforeModel: (transition) ->
    @_super.apply this, arguments
    Em.$.ajax(
      type: 'DELETE'
      url: "/api/v0/logout"
      cache: false
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    ).fail (jqxhr) =>
      App.postError(jqxhr.responseJSON, 'Failed to request logout!')

    App.set 'token', null
    Em.run.next ->
      confirmationLapsed = transition?.queryParams.confirmationLapsed == true
      App.__container__.lookup('router:main').transitionTo 'login', queryParams: { confirmationLapsed: confirmationLapsed }

App.LogoutRoute = Em.Route.extend
  beforeModel: (transition) ->
    @_super.apply this, arguments

    App.logout().then ->
      Em.run.next ->
        # exit app, do not reuse app after logout, because there is no reliable way
        # to clear old app state (e.g. models, mixpanel etc.)
        location.href = "/"

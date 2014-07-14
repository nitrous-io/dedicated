App.Router.map ->
  @route 'login'

  @resource 'home', path: '/', ->
    @resource 'container', path: '/containers/:container_id', ->

App.Router.map ->
  @route 'login'

  @resource 'home', path: '/', ->
    @resource 'containers', ->
      @route 'new'

    @resource 'container', path: '/containers/:container_id', ->
      @route 'keys'
      @route 'keys-new', path: '/keys/new'

    @resource 'hosts', ->
      @route 'new'

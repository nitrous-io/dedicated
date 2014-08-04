App.Router.map ->
  @route 'signup'
  @route 'login'
  @route 'logout'

  @resource 'home', path: '/', ->
    @resource 'containers', ->
      @route 'new'

    @resource 'container', path: '/containers/:container_id', ->
      @route 'keys'
      @route 'keys-new', path: '/keys/new'
      @route 'terminal'

    @resource 'hosts', ->
      @route 'new'

    @resource 'host', path: '/hosts/:host_id', ->
      @route 'registries'
      @route 'registries-new', path: '/registries/new'
      @route 'volumes'
      @route 'volumes-new', path: '/volumes/new'

    @resource 'images', ->
      @route 'new'

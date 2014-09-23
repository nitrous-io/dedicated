App.Router.map ->
  @route 'signup'
  @route 'login'
  @route 'logout'

  @resource 'home', path: '/', ->
    @resource 'containers', ->
      @route 'new'

    @resource 'container', path: '/containers/:containerSlug', ->
      @route 'keys'
      @route 'keys-new', path: '/keys/new'
      @route 'terminal'

    @resource 'hosts', ->
      @route 'new'

    @resource 'host', path: '/hosts/:hostSlug', ->
      @route 'registries'
      @route 'registries-new', path: '/registries/new'
      @route 'images'
      @route 'volumes'
      @route 'volumes-new', path: '/volumes/new'

    @resource 'images', ->
      @route 'new'
      @route 'pull'

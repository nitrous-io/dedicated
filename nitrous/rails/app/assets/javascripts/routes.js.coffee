App.Router.map ->
  @route 'signup'
  @route 'login'
  @route 'logout'
  @route 'forgot-password'
  @route 'reset-password'
  @route 'confirm-email'

  @resource 'home', path: '/', ->
    @resource 'containers', ->
      @route 'new'

    @resource 'container', path: '/containers/:containerSlug', ->
      @route 'keys'
      @route 'keys-new', path: '/keys/new'
      @route 'config'
      @route 'terminal'

    @resource 'hosts', path: '/applications', ->
      @route 'new'

    @resource 'host', path: '/applications/:hostSlug', ->
      @route 'registries'
      @route 'registries-new', path: '/registries/new'
      @route 'images'
      @route 'volumes'
      @route 'volumes-new', path: '/volumes/new'

    @resource 'images', ->
      @route 'new'
      @route 'pull'

    @resource 'account'
    @resource 'billing'

App.Router.map ->
  @route 'signup'
  @route 'login'
  @route 'logout'
  @route 'forgot-password'
  @route 'reset-password'
  @route 'confirm-email'
  @route 'pricing'

  @resource 'home', path: '/', ->
    @resource 'containers', path: '/applications', ->
      @route 'new'

    @resource 'container', path: '/applications/:containerSlug', ->
      @route 'system'
      @route 'keys'
      @route 'keys-new', path: '/keys/new'
      @route 'config'
      @route 'ports'
      @route 'console'

    @resource 'hosts', path: '/workspaces', ->
      @route 'new'

    @resource 'host', path: '/workspaces/:hostSlug', ->
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
    @resource 'add-card'

  @resource 'admin', ->
    @route 'hosts', path: 'hosts/:provider'

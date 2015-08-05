App.Router.map ->
  @route 'signup'
  @route 'login'
  @route 'logout'
  @route 'forgot-password'
  @route 'reset-password'
  @route 'confirm-email'
  @route 'pricing'
  @route 'coupon', path: '/claim'

  @resource 'home', path: '/', ->
    @resource 'free-tier', path: '/free-tier'

    @resource 'containers', path: '/containers', ->
      @route 'new'

    @resource 'subscriptions', path: '/subscriptions/:slug', ->
      @route 'upgrade'

    @resource 'container', path: '/containers/:containerSlug', ->
      @route 'collaborators'
      @route 'keys'
      @route 'keys-new', path: '/keys/new'
      @route 'config'
      @route 'ports'
      @route 'console'
      @route 'custom-domain'

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

    @resource 'orgs', path: '/orgs', ->
      @route 'new'

    @resource 'org', path: '/orgs/:orgSlug', ->

    @resource 'account'
    @resource 'add-phone'
    @resource 'billing'
    @resource 'add-card'
    @resource 'provider-templates', path: '/provider-templates'
    @resource 'provider-template', path: '/provider-templates/:ptid'

    @resource 'admin', path: '/admin', ->
      @route 'amazon-hosts', path: '/amazon/hosts'
      @route 'hosts', path: '/hosts/:provider'
      @route 'templates', path: '/templates'
      @route 'templates-new', path: '/templates/new'
      @route 'template-edit', path: '/template/:templateSlug/edit'
      @route 'announcements', path: '/announcements'
      @route 'announcements-new', path: '/announcements/new'
      @route 'user', path: '/users/:slug'

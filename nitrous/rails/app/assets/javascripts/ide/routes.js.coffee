App.Router.map ->
  @route 'blank'
  @resource 'ide-upgrade', path: '/:containerSlug/upgrade'
  @resource 'ide', path: '/:containerSlug'

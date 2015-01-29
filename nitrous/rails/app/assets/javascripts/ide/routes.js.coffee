App.Router.map ->
  @route 'blank'
  @resource 'ide', path: '/:containerSlug'
  @resource 'terminal', path: '/:containerSlug/terminal'

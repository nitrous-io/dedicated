App.BoxesNewRoute = Em.Route.extend
  queryParams:
    hack_repo_url:
      refreshModel: true

  model: ->
    App.Box.create
      memory: App.config.minMemoryMB
      storage: App.config.minStorageMB

  activate: ->
    App.get('currentUser').fetchN2O()
    App.get('snapshots').reload()
    App.get('boxes').reload()

  afterModel: (model, transition) ->
    # FIXME Figure out how to share this code with IntroCreateBoxRoute.
    # The problem is that we can't do this in setupController() because the
    # queryParams are not yet set at that point. If we can find the correct
    # hook to set these model properties from the queryParams, we can move this
    # into a controller method and re-use that.
    model.set 'repository_clone_url', transition.queryParams.hack_repo_url
    model.set 'box_template_id', App.get('boxTemplates.firstObject.id')

    if runtime = transition.queryParams.hack_runtime
      boxTemplate = App.get('boxTemplates').findBy('runtime', runtime)
      if boxTemplate?
        model.set 'box_template_id', boxTemplate.get('id')

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.set 'repo', App.Repository.create()

  deactivate: ->
    @controller.set 'content', null

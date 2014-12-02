App.IntroCreateBoxRoute = Em.Route.extend
  queryParams:
    hack_repo_url:
      refreshModel: true

  controllerName: 'boxesNew'

  model: ->
    App.Box.create()

  activate: ->
    App.get('currentUser').fetchN2O()

  afterModel: (model, transition) ->
    model.set 'repository_clone_url', transition.queryParams.hack_repo_url
    model.set 'box_template_id', App.get('boxTemplates.firstObject.id')

    if runtime = transition.queryParams.hack_runtime
      boxTemplate = App.get('boxTemplates').findBy('runtime', runtime)
      if boxTemplate?
        model.set 'box_template_id', boxTemplate.get('id')

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.set 'repo', App.Repository.create()
    controller.set 'isIntro', true

  renderTemplate: ->
    @render 'intro/create_box', controller: 'boxesNew'

  deactivate: ->
    @controllerFor('boxesNew').setProperties(isIntro: false, content: null)

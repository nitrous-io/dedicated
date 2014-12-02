#= require ./ide-route

App.BoxIdeRoute = App.IdeRoute.extend
  repository_id: null

  afterModel: (model, transition) ->
    @set 'repository_id', transition.queryParams.repository_id

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.initialize()
    if repository_id = @get 'repository_id'
      controller.set 'repository', App.Repository.findById(parseInt repository_id, 10)

  deactivate: ->
    @_super()
    @set 'repository_id', null
    @controller.reset()

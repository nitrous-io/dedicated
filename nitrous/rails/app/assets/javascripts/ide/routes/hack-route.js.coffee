App.HackRoute = Em.Route.extend
  queryParams:
    hack_repo_url:
      refreshModel: true

  actions:
    upgrade: ->
      @transitionTo 'n2o.upgrade'

  model: ->
    App.Repository.create()

  activate: ->
    App.util.saveReturnTo('hack')

  afterModel: (model, transition) ->
    model.set 'clone_url', transition.queryParams.hack_repo_url

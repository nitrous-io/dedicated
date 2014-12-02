App.BoxSnapshotsIndexRoute = Em.Route.extend
  model: ->
    App.get 'snapshots'

  activate: ->
    App.get('snapshots').reload()

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.set 'box', @modelFor 'box'

  deactivate: ->
    @controller.setProperties box: null, content: null

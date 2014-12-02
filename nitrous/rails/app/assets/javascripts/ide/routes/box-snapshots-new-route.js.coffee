App.BoxSnapshotsNewRoute = Em.Route.extend
  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.set 'box', @modelFor 'box'
    controller.newRecord()

  deactivate: ->
    @controller.setProperties box: null, content: null

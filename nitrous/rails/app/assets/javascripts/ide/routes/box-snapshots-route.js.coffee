App.BoxSnapshotsRoute = Em.Route.extend
  beforeModel: ->
    unless App.get('currentUser.box_snapshots_allowed')
      @transitionTo 'boxes.index'

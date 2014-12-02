App.BoxSnapshotsIndexController = Em.ArrayController.extend
  contentBinding: 'box.snapshots'
  box: null

  newSnapshot: ->
    App.get('router').transitionTo 'box.snapshots.new', @get('box')

  destroy: (snapshot) ->
    snapshot.deleteRecord()

  cancel: ->
    App.get('router').transitionTo 'boxes.index'

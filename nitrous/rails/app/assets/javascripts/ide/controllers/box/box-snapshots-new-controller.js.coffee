App.BoxSnapshotsNewController = Em.ObjectController.extend
  box: null

  newRecord: ->
    snapshot = App.Snapshot.create()
    snapshot.set 'box_id', @get 'box.id'
    snapshot.set 'name', "#{@get('box.name')}-#{App.util.strftime('%Y%m%d-%H%M')}"
    @set 'content', snapshot

  save: ->
    record = @get 'content'
    record.save().then =>
      App.get('router').transitionTo 'box.snapshots.index', @get('content.box')

  cancel: ->
    App.get('router').transitionTo 'box.snapshots.index', @get('content.box')

  shutdownBox: ->
    @get('box').stop()

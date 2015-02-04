App.ContainerIndexController = Em.ObjectController.extend App.PerformActionMixin,
  snapshotImageName: null
  closeSnapshotDialog: false
  snapshotting: false

  clear: ->
    setProperties:
      snapshotImageName: null
      closeSnapshotDialog: false
      snapshotting: false

  performActionWithTempState: (method, action, tempState, data=null)  ->
    @get('model').setTempState tempState
    @performAction method, action, tempState

  actions:
    start: ->
      @performActionWithTempState('POST', 'start', 'starting')

    restart: ->
      @performActionWithTempState('POST', 'restart', 'restarting')

    stop: ->
      @performActionWithTempState('POST', 'stop', 'stopping')

    terminate: ->
      name = @get 'model.displayName'
      @performActionWithTempState('DELETE', null, 'terminating').then ->
        App.snackbar.post("Your workspace “#{name}” was successfully terminated.")

    snapshot: ->
      name = @get 'snapshotImageName'
      record = @get 'model'
      @set 'snapshotting', true
      record.ajax('POST', 'commit', name: name).then(=>
        App.snackbar.post("Your snapshot “#{name}” was successfully created.")
        @set 'closeSnapshotDialog', true
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, "Failed to snapshot!")
      ).finally =>
        @set 'snapshotting', false

    openLogs: ->
      @transitionToRoute 'container.console', queryParams: { action: 'logs' }

    openAttach: ->
      @transitionToRoute 'container.console', queryParams: { action: 'attach' }

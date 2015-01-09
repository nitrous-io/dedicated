App.ContainerIndexController = Em.ObjectController.extend App.PerformActionMixin,
  snapshotImageName: null
  closeSnapshotDialog: false
  snapshotting: false

  clear: ->
    setProperties:
      snapshotImageName: null
      closeSnapshotDialog: false
      snapshotting: false

  actions:
    start: ->
      @performAction('POST', 'start', 'starting')

    startApp: ->
      @performAction('POST', 'start_app')

    stopApp: ->
      @performAction('POST', 'stop_app')

    restartApp: ->
      @performAction('POST', 'restart_app')

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

    openTerminal: ->
      @transitionToRoute 'container.console', queryParams: { action: null }

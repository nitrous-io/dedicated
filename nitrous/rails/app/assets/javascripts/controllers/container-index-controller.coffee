App.ContainerIndexController = Em.ObjectController.extend App.PerformActionMixin,
  snapshotImageName: null
  closeSnapshotDialog: false
  snapshotting: false

  openingIDE: false

  clear: ->
    setProperties:
      snapshotImageName: null
      closeSnapshotDialog: false
      snapshotting: false
      openingIDE: false

  actions:
    start: ->
      @performAction('POST', 'start', 'starting')

    restart: ->
      @performAction('POST', 'restart', 'restarting')

    stop: ->
      @performAction('POST', 'stop', 'stopping')

    terminate: ->
      name = @get 'model.displayName'
      hostSlug = @get 'model.host.slug'
      @performAction('DELETE', null, 'terminating').then =>
        App.snackbar.post("Your container “#{name}” was successfully terminated.")
        @transitionToRoute 'host.index', hostSlug

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

    openIDE: ->
      @set 'openingIDE', true
      window.location.href = @get 'model.ideURL'

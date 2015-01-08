App.ContainerSystemController = Em.ObjectController.extend App.PerformActionMixin,
  actions:
    start: ->
      @performAction('POST', 'start', 'starting')

    restart: ->
      @performAction('POST', 'restart', 'restarting')

    stop: ->
      @performAction('POST', 'stop', 'stopping')

    terminate: ->
      name = @get 'model.displayName'
      @performAction('DELETE', null, 'terminating').then ->
        App.snackbar.post("Your workspace “#{name}” was successfully terminated.")

    openLogs: ->
      @transitionToRoute 'container.console', queryParams: { action: 'logs' }

    openAttach: ->
      @transitionToRoute 'container.console', queryParams: { action: 'attach' }

App.ContainerIndexController = Em.ObjectController.extend App.PerformActionMixin,
  actions:
    start: ->
      @performAction('POST', 'start', 'starting')

    restart: ->
      @performAction('POST', 'restart', 'restarting')

    stop: ->
      @performAction('POST', 'stop', 'stopping')

    terminate: ->
      name = @get('model.displayName')
      @performAction('DELETE', null, 'terminating').then ->
        App.snackbar.post("Your container “#{name}” was successfully terminated.")

    openLogs: ->
      @transitionToRoute 'container.terminal', queryParams: { action: 'logs' }

    openAttach: ->
      @transitionToRoute 'container.terminal', queryParams: { action: 'attach' }

    openTerminal: ->
      @transitionToRoute 'container.terminal'

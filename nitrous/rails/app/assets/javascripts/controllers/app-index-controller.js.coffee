#= require ../mixins/perform-action-mixin

App.AppIndexController = Em.ObjectController.extend App.PerformActionMixin,
  actions:
    start: ->
      hostState = @get 'model.state'
      container = @get 'model.appContainer'

      if hostState != 'running'
        @performAction('POST', 'restart', 'restarting')
      else if container
        @performActionOnRecord(container, 'POST', 'start', 'starting')

    restart: ->
      if container = @get 'model.appContainer'
        @performActionOnRecord(container, 'POST', 'restart', 'restarting')
      else
        @performAction('POST', 'restart', 'restarting')

    stop: ->
      if container = @get 'model.appContainer'
        @performActionOnRecord(container, 'POST', 'stop', 'stopping')

    terminate: ->
      name = @get('model.displayName')
      @performAction('DELETE', null, 'terminating').then ->
        App.snackbar.post("Your application “#{name}” was successfully terminated.")

    refresh: ->
      @get('model').fetchStats()

    openLogs: ->
      @transitionToRoute 'app.console', queryParams: { action: 'logs' }

    openTerminal: ->
      @transitionToRoute 'app.console', queryParams: { action: null }

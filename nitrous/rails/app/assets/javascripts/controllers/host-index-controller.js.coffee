#= require ../mixins/perform-action-mixin

App.HostIndexController = Em.ObjectController.extend App.PerformActionMixin,
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
        App.snackbar.post("Your host “#{name}” was successfully terminated.")

    refresh: ->
      @get('model').fetchStats()

    newContainer: ->
      @transitionToRoute 'containers.new', queryParams: { preselectHost: @get('model.slug') }

#= require ../mixins/perform-action-mixin

App.HostIndexController = Em.ObjectController.extend App.PerformActionMixin,
  actions:
    restart: ->
      @performAction('POST', 'restart', 'restarting')

    terminate: ->
      name = @get('model.displayName')
      @performAction('DELETE', null, 'terminating').then ->
        App.snackbar.post("Your workspace “#{name}” was successfully terminated.")

    refresh: ->
      @get('model').fetchStats()

    newContainer: ->
      @transitionToRoute 'containers.new', queryParams: { preselectHost: @get('model.slug') }

    upgrade: ->
      @performAction('POST', 'upgrade', 'upgrading')

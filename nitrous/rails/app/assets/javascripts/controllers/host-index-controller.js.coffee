#= require ../mixins/perform-action-mixin

App.HostIndexController = Em.ObjectController.extend App.PerformActionMixin,
  stateDidChange: (->
    if @get('model.state') == 'running'
      Em.run.later(this, ->
        @send('refresh')
      , 1000)
  ).observes 'model.state'

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
        App.snackbar.post("Your workspace “#{name}” was successfully terminated.")

    refresh: ->
      @get('model').fetchStats()

    newContainer: ->
      @transitionToRoute 'containers.new', queryParams: { preselectHost: @get('model.slug') }

    upgrade: ->
      host = @get('model')
      events = App.get 'eventsSocket'

      unless host.get('canUpgrade')
        App.postError("Containers must be stopped to perform this upgrade")
        return

      onUpgradeFailed = (data) ->
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.upgrade_failed', onUpgradeFailed
        events.off 'host.upgraded', onUpgraded
        App.postError("Failed to upgrade workspace!")

      onUpgraded = (data) ->
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.upgrade_failed', onUpgradeFailed
        events.off 'host.upgraded', onUpgraded
        App.snackbar.post("Your workspace “#{host.get('slug')}” has been upgraded.")

      events.on 'host.upgrade_failed', onUpgradeFailed
      events.on 'host.upgraded', onUpgraded

      @performAction('POST', 'upgrade', 'upgrading')

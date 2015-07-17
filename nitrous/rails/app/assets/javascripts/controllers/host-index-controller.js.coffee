#= require ../mixins/perform-action-mixin

App.HostIndexController = Em.ObjectController.extend App.PerformActionMixin,
  queryParams: ['justUpgraded']
  justUpgraded: false

  actions:
    start: ->
      @performAction('POST', 'start', 'starting')

    restart: (opt = {})->
      @performAction('POST', 'restart', 'restarting', opt)

    stop: (opt = {}) ->
      @performAction('POST', 'stop', 'stopping', opt)

    terminate: ->
      name = @get('model.displayName')
      state = 'terminating'
      if @get('model.canCancelSubscription')
        state = null
      @performAction('DELETE', null, state).then (data)=>
        if data.subscription
          @set 'model.subscription.cancelPending', true
          App.snackbar.post("Subscription for workspace “#{name}” was successfully cancelled.")
          return
        App.snackbar.post("Your workspace “#{name}” was successfully terminated.")

    refresh: ->
      @get('model').fetchStats()

    newContainer: ->
      @transitionToRoute 'containers.new', queryParams: { preselectHost: @get('model.slug') }

    upgradePaid: ->
      subscriptionSlug = @get('model.subscriptionSlug')
      @transitionToRoute 'subscriptions.upgrade', subscriptionSlug

    update: ->
      host = @get('model')
      events = App.get('eventsSocket')

      unless host.get('canUpdate')
        App.postError('Containers must be stopped to perform this update.')
        return

      onUpdateFailed = (data) ->
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.update_failed', onUpdateFailed
        events.off 'host.updated', onUpdated
        App.postError('Failed to update workspace!')

      onUpdated = (data) ->
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.update_failed', onUpdateFailed
        events.off 'host.updated', onUpdated
        App.snackbar.post("Your workspace “#{host.get('slug')}” has been updated.")

      events.on 'host.update_failed', onUpdateFailed
      events.on 'host.updated', onUpdated

      @performAction('POST', 'update_software', 'updating')

    respec: ->
      host = @get('model')
      events = App.get('eventsSocket')

      onRespecFailed = (data) ->
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.respec_failed', onRespecFailed
        events.off 'host.respecced', onRespecced
        App.postError('Failed to upgrade workspace!')

      onRespecced = (data) ->
        hostData = data.host
        return unless hostData.id == host.get('id')
        events.off 'host.respec_failed', onRespecFailed
        events.off 'host.respecced', onRespecced
        App.snackbar.post("Your workspace “#{host.get('slug')}” has been upgraded.")

      events.on 'host.respec_failed', onRespecFailed
      events.on 'host.respecced', onRespecced

      @performAction('POST', 'respec', 'respeccing')

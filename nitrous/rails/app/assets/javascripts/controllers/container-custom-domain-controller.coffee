App.ContainerCustomDomainController = Em.ObjectController.extend App.PerformActionMixin,
  enabling: false
  prorataCharge: null

  # Whether we can enable Hosting Mode for this container's workspace.
  canEnable: (->
    return false if @get('host.free')
    return true if !@get('host.autoShutdown')
    @get('host.plan.hostingModePlan')
  ).property 'host.free', 'host.autoShutdown', 'host.plan.hostingModePlan'

  computeProrataCharge: ->
    return if !@get('host.autoShutdown')

    hostingModePlan = @get('host.plan.hostingModePlan')
    return if !hostingModePlan

    @get('host.subscription').upgradeEstimate(hostingModePlan.get('id')).then (data) =>
      if data.total != null
        @set('prorataCharge', (data.total/100.0))

  clear: ->
    @_super.apply this, arguments
    @setProperties
      enabling: false
      prorataCharge: null
    @computeProrataCharge()

  actions:
    upgrade: ->
      @transitionToRoute 'subscriptions.upgrade', @get('host.subscriptionSlug')

    enableHostingMode: ->
      host = @get('host')
      subscription = host.get('subscription')
      newPlanId = @get('host.plan.hostingModePlan.id')

      subscription.upgrade(newPlanId).then (data) =>
        events = App.get('eventsSocket')

        onRespecFailed = (data) =>
          hostData = data.host
          return unless hostData.id == host.get('id')
          events.off 'host.respec_failed', onRespecFailed
          events.off 'host.respecced', onRespecced
          @set('enabling', false)
          App.postError('Failed to enable Hosting Mode. Please contact support@nitrous.io.')

        onRespecced = (data) =>
          hostData = data.host
          return unless hostData.id == host.get('id')
          events.off 'host.respec_failed', onRespecFailed
          events.off 'host.respecced', onRespecced
          @set('enabling', false)
          App.snackbar.post("Hosting Mode has been enabled on your “#{host.get('slug')}” workspace.")

        events.on 'host.respec_failed', onRespecFailed
        events.on 'host.respecced', onRespecced
        @performActionOnRecord(host, 'POST', 'respec', 'respeccing')
        @set('enabling', true)

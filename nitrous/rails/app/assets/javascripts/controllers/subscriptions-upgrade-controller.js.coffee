App.SubscriptionsUpgradeController = Em.ObjectController.extend App.PerformActionMixin,
  selectedPlan: null
  prorataCharge: null

  upgrading: false
  hostsUpgrading: null
  manyHostsUpgrading: false

  paidPlans: (->
    curPlan = @get('model.plan')
    App.get('eligiblePlans').filter (plan) ->
      plan.get('isPaidPlan') && !plan.get('hidden') && (plan.get('price') > curPlan.get('price'))
  ).property 'App.eligiblePlans.@each', 'model.plan'

  upgradeSubscriptionDisabled: (->
    @get('selectedPlan') == @get('model.plan')
  ).property 'selectedPlan', 'model.plan'

  difference: (->
    (@get('selectedPlan.totalPrice') - @get('model.plan.totalPrice')) / 100.0
  ).property 'selectedPlan', 'model.plan'

  computeFutureCharge: (->
    @set 'prorataCharge', null
    return unless @get('model') && @get('selectedPlan')
    return if @get('model.plan') == @get('selectedPlan')
    id = @get('selectedPlan.id')
    @get('model').upgradeEstimate(id).then (data) =>
      if data.total != null && id == @get 'selectedPlan.id'
        @set 'prorataCharge', (data.total/100.0)
  ).observes 'selectedPlan', 'model.plan'

  clear: ->
    @setProperties
      upgrading: false,
      prorataCharge: null,
      hostsUpgrading: null,
      manyHostsUpgrading: false

    @set 'selectedPlan', @get('model.plan')

  actions:
    performUpgrade: ->
      @set 'upgrading', true
      events = App.get('eventsSocket')

      @get('model').upgrade(@get('selectedPlan.id')).then (data)=>
        hosts = App.get('hosts')?.filterBy('subscriptionId', @get('model.id'))
        @set 'hostsUpgrading', hosts
        @set 'manyHostsUpgrading', @get('hosts.length') > 1

        currentModal = $('#subscription-upgrade-modal-dialog')[0]

        # perform respec
        hosts.forEach (host)=>
          onRespecFailed = (data) ->
            hostData = data.host
            return unless hostData.id == host.get('id')
            events.off 'host.respec_failed', onRespecFailed
            events.off 'host.respecced', onRespecced
            App.postError('Failed to upgrade workspace!')

          onRespecced = (data) =>
            hostData = data.host
            return unless hostData.id == host.get('id')
            events.off 'host.respec_failed', onRespecFailed
            events.off 'host.respecced', onRespecced
            ui.hideModal currentModal
            @transitionToRoute 'host.index', host.get 'slug'
            App.snackbar.post("Your workspace “#{host.get('slug')}” has been upgraded.")

          events.on 'host.respec_failed', onRespecFailed
          events.on 'host.respecced', onRespecced
          @performActionOnRecord(host, 'POST', 'respec', 'respeccing')

        @set 'upgrading', false
        unless @get 'hostsUpgrading.length'
          ui.hideModal currentModal
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Unable to upgrade!')
        @set 'upgrading', false

    cancel: ->
      @transitionToRoute 'billing'


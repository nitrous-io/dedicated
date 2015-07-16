App.SubscriptionsUpgradeController = Em.ObjectController.extend App.PerformActionMixin,
  queryParams: ['pendingPlanId']
  selectedPlan: null
  prorataCharge: null

  upgrading: false
  hostsUpgrading: null
  manyHostsUpgrading: false

  # when being transferred from successful add-card
  pendingPlanId: null

  paidPlans: (->
    return [] unless @get 'model.plan'

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

    sPlan = null
    if (pl = @get 'pendingPlanId')
      sPlan = @get('paidPlans').findBy('id', parseInt(pl))

    @set 'selectedPlan', (sPlan || @get('model.plan'))

    if sPlan
      Em.run.next ->
        ui.showModal '#subscription-upgrade-modal-dialog'
      window.setTimeout(=>
        @send 'performUpgrade'
      , 10)

  actions:
    performUpgrade: ->
      if App.get('billingProfile.isFreeOrNoCardState')
        @transitionToRoute 'add-card', queryParams: { subscriptionId: @get('model.id'), pendingPlanId: @get('selectedPlan.id') }
        return

      # wipe for accidental restore
      @set 'pendingPlanId', null
      @set 'upgrading', true
      events = App.get('eventsSocket')

      @get('model').upgrade(@get('selectedPlan.id')).then (data)=>
        hosts = App.get('hosts')?.filterBy('subscriptionSlug', @get('model.slug'))
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


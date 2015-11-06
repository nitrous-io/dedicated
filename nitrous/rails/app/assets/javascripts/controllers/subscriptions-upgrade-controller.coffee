App.SubscriptionsUpgradeController = Em.ObjectController.extend App.PerformActionMixin,
  needs: ['orgs']
  queryParams: ['planId', 'exceededContainerLimit', 'selectedTemplate', 'containerName']
  planId: null # when being transferred from successful add-card
  exceededContainerLimit: false
  selectedTemplate: null
  containerName: null

  selectedPlan: null
  prorataCharge: null

  upgrading: false
  hostsUpgrading: null
  manyHostsUpgrading: false

  orgPlans: null

  availablePlans: (->
    return [] unless @get 'model.plan'
    curPlan = @get('model.plan')
    plans = App.get('plans')

    if @get('model.orgId')
      if @get('orgPlans')
        plans = @get('orgPlans')
      else
        return []

    plans.filter (plan) ->
      plan.get('isPaidPlan') && !plan.get('hidden') && (plan.get('price') > curPlan.get('price'))
  ).property 'App.plans.@each', 'model.plan', 'orgPlans'

  updateOrgPlans: ( ->
    if @get('model.orgId')
      App.OrgPlan.fetch(orgSlug: @get('model.orgId')).then (plans) =>
        @set 'orgPlans', plans
  ).observes 'model.orgId'

  upgradeSubscriptionDisabled: (->
    @get('selectedPlan') == @get('model.plan')
  ).property 'selectedPlan', 'model.plan'

  difference: (->
    (@get('selectedPlan.totalPrice') - @get('model.plan.totalPrice')) / 100.0
  ).property 'selectedPlan', 'model.plan'

  computeProrataCharge: (->
    @set 'prorataCharge', null
    return unless @get('model') && @get('selectedPlan')
    return if @get('model.plan') == @get('selectedPlan')
    @get('model').upgradeEstimate(@get('selectedPlan.id')).then (data) =>
      if data.total != null
        @set 'prorataCharge', (data.total/100.0)
  ).observes 'selectedPlan', 'model.plan'

  clear: ->
    @setProperties
      upgrading: false,
      prorataCharge: null,
      hostsUpgrading: null,
      manyHostsUpgrading: false

    automateUpgrade = false
    selectedPlan = @get('model.plan')
    if (pl = @get 'planId')
      automateUpgrade = true
      selectedPlan = @get('availablePlans').findBy('id', parseInt(pl))

    @set 'selectedPlan', selectedPlan

    if automateUpgrade && selectedPlan
      Em.run.next =>
        ui.showModal '#subscription-upgrade-modal-dialog'
        window.setTimeout(=>
          @send 'performUpgrade'
        , 10)

  actions:
    performUpgrade: ->
      if App.get('billingProfile.isFreeOrNoCardState')
        @transitionToRoute 'add-card', queryParams: { subscriptionSlug: @get('model.slug'), pendingPlanId: @get('selectedPlan.id') }
        return

      # wipe to prevent re-navigating to the route restarting an upgrade
      @set 'planId', null
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
            if @get('exceededContainerLimit') && tmplSlug = @get 'selectedTemplate'
              @transitionToRoute 'containers.new', queryParams: { preselectHost: host.get('slug'), selectedTemplateSlug: tmplSlug, autofillName: @get('containerName') }
            else
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
      if orgId = @get('model.orgId')
        @transitionToRoute 'org.billing', @get('controllers.orgs.collection').findBy('id', orgId).get('slug')
      else
        @transitionToRoute 'billing'

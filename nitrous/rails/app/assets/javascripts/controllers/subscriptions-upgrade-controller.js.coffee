App.SubscriptionsUpgradeController = Em.ObjectController.extend
  selectedPlan: null
  prorataCharge: null
  upgrading: false

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
      prorataCharge: null

    @set 'selectedPlan', @get('model.plan')

  actions:
    performUpgrade: ->
      @set 'upgrading', true
      @get('model').upgrade(@get('selectedPlan.id')).then (data)=>
        hosts = App.get('hosts')?.filterBy('subscriptionId', @get('model.id'))
        first = hosts.get 'firstObject'

        @set 'upgrading', false
        ui.hideModal '#subscription-upgrade-modal-dialog'
        @transitionToRoute 'host.index', first.get 'slug'
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Unable to upgrade!')
        @set 'upgrading', false
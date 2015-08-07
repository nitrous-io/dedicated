App.AddCardRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Billing'

    @subBillingAddCardUpgradedToPaid = PubSub.subscribe 'billing.addCard.upgradedToPaid', (msg, data) =>
      @controllerFor('addCard').setProperties
        subscriptionSlug: null
        pendingPlanId: null
      @transitionTo 'subscriptions.upgrade', data.subscriptionSlug, queryParams: { planId: data.planId }

    @subBillingAddCardAdded = PubSub.subscribe 'billing.addCard.added', (msg, pendingHost) =>
      if pendingHost
        @transitionTo 'hosts.new'
      else
        @transitionTo 'billing'

    @subBillingAddCardCanceled = PubSub.subscribe 'billing.addCard.canceled', (msg, pendingHost) =>
      if pendingHost
        App.set 'pendingHost', null
        @transitionTo 'hosts.new'
      else
        @transitionTo 'billing'

  deactivate: ->
    PubSub.unsubscribe @subBillingAddCardUpgradedToPaid
    PubSub.unsubscribe @subBillingAddCardAdded
    PubSub.unsubscribe @subBillingAddCardCanceled

  renderTemplate: ->
    @render 'add-card', outlet: 'content'

  resetController: (controller, isExiting, transition) ->
    if isExiting
      # Reset query params so they're not sticky.
      controller.set('subscriptionSlug', null)
      controller.set('pendingPlanId', null)

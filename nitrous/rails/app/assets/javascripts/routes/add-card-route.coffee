App.AddCardRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Billing'

    @subAddCardUpgradedToPaid = PubSub.subscribe 'addCard.upgradedToPaid', (msg, data) =>
      @controllerFor('addCard').setProperties
        subscriptionSlug: null
        pendingPlanId: null
      @transitionTo 'subscriptions.upgrade', data.subscriptionSlug, queryParams: { planId: data.planId }

    @subAddCardAdded = PubSub.subscribe 'addCard.added', (msg, pendingHost) =>
      if pendingHost
        @transitionTo 'hosts.new'
      else
        @transitionTo 'billing'

    @subAddCardCanceled = PubSub.subscribe 'addCard.canceled', (msg, pendingHost) =>
      if pendingHost
        App.set 'pendingHost', null
        @transitionTo 'hosts.new'
      else
        @transitionTo 'billing'

  deactivate: ->
    PubSub.unsubscribe @subAddCardUpgradedToPaid
    PubSub.unsubscribe @subAddCardAdded
    PubSub.unsubscribe @subAddCardCanceled

  renderTemplate: ->
    @render 'add-card', outlet: 'content'

  resetController: (controller, isExiting, transition) ->
    if isExiting
      # Reset query params so they're not sticky.
      controller.set('subscriptionSlug', null)
      controller.set('pendingPlanId', null)

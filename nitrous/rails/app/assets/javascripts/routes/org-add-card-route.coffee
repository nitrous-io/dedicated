#= require ./authed-route

App.OrgAddCardRoute = App.AuthedRoute.extend
  controllerName: 'org'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.set 'currentTab', 'add-card'
    model = controller.get 'model'
    App.set 'title', model.get('displayName') + ' › Billing › Add Card'

    @subBillingAddCardAdded = PubSub.subscribe 'billing.addCard.added', (msg) =>
      @transitionTo 'org.billing'

    @subBillingAddCardCanceled = PubSub.subscribe 'billing.addCard.canceled', (msg) =>
      @transitionTo 'org.billing'

  deactivate: ->
    @controller.set 'currentTab', null
    PubSub.unsubscribe @subBillingAddCardAdded
    PubSub.unsubscribe @subBillingAddCardCanceled

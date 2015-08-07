#= require ./authed-route

App.OrgBillingRoute = App.AuthedRoute.extend
  controllerName: 'org'

  activate: ->
    @subOrgBillingWantToAddCard = PubSub.subscribe 'org.billing.wantToAddCard', (msg) =>
      @transitionTo 'org.add-card'

  deactivate: ->
    @controller.set 'currentTab', null
    PubSub.unsubscribe @subOrgBillingWantToAddCard

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.set 'currentTab', 'billing'
    model = controller.get 'model'
    App.set 'title', model.get('displayName') + ' › Billing'

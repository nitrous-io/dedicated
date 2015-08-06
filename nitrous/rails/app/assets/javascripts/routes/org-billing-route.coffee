#= require ./authed-route

App.OrgBillingRoute = App.AuthedRoute.extend
  controllerName: 'org'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.set 'currentTab', 'billing'
    model = controller.get 'model'
    App.set 'title', model.get('displayName') + ' › Billing'

  deactivate: ->
    @controller.set 'currentTab', null

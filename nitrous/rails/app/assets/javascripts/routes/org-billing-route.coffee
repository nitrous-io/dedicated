#= require ./authed-route

App.OrgBillingRoute = App.AuthedRoute.extend
  controllerName: 'org'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.set 'currentTab', 'billing'

  deactivate: ->
    console.log @controller
    @controller.set 'currentTab', null

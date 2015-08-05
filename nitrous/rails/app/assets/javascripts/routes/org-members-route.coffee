#= require ./authed-route

App.OrgMembersRoute = App.AuthedRoute.extend
  controllerName: 'org'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.set 'currentTab', 'members'

  deactivate: ->
    console.log @controller
    @controller.set 'currentTab', null

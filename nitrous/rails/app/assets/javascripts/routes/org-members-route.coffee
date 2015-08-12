#= require ./authed-route

App.OrgMembersRoute = App.AuthedRoute.extend
  controllerName: 'org'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.set 'currentTab', 'members'
    model = controller.get 'model'
    App.set 'title', model.get('displayName') + ' › Members'

  deactivate: ->
    @controller.set 'currentTab', null

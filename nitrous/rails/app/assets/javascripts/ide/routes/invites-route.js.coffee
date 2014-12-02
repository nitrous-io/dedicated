App.InvitesRoute = Em.Route.extend
  model: ->
    App.get 'invites'

  activate: ->
    App.get('invites').reload()

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.set 'newInvite', App.Invite.create()

  deactivate: ->
    @controller.setProperties content: null, newInvite: null

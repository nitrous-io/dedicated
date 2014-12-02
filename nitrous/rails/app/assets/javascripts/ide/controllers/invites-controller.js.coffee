App.InvitesController = Em.ArrayController.extend
  sortProperties: ['created_at']
  sortAscending: false

  newInvite: null

  sendInvite: ->
    @get('newInvite')?.save().then =>
      @set 'newInvite', App.Invite.create()

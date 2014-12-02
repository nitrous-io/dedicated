App.InvitesCollection = Em.ArrayProxy.extend
  content: null

App.CollaboratorsCollection = Em.ArrayProxy.extend
  content: null

App.ManageCollaboratorsDialog = Em.Object.extend
  collaborators: null # array of App.Collaborator
  invites: null # instance of App.InvitesCollection
  boxBinding: Em.Binding.oneWay 'App.router.boxIdeController.content'

  newCollaboratorIdentifier: ''
  errorMessage: null
  saving: false

  init: ->
    @set 'collaborators', Em.A()
    @set 'invites', Em.A()
    @_super.apply this, arguments

  fetch: ->
    box = @get 'box'
    App.Collaborator.fetch box.get('id'), (err, records) =>
      @set 'collaborators', App.CollaboratorsCollection.create(content: Em.A(records))
    App.CollabInvite.fetch box.get('id'), (err, records) =>
      @set 'invites', App.InvitesCollection.create(content: Em.A(records))

  addCollaborator: (done) ->
    identifier = $.trim @get('newCollaboratorIdentifier')
    @set 'newCollaboratorIdentifier', identifier
    return unless identifier.length > 0

    @set 'saving', true
    box = @get 'box'
    collaborators = @get 'collaborators'
    invites = @get 'invites'

    $.ajax(
      type: 'POST'
      dataType: 'json'
      data:
        identifier: identifier
      url: "/boxes/#{box.get('id')}/collaborators"
    ).done((json) =>
      if json
        if json.collaborator?
          record = App.Collaborator.create()
          record.loadHash json.collaborator
          collaborators.addObject record
        else if json.invite?
          record = App.CollabInvite.create()
          record.loadHash json.invite
          invites.addObject record
      @set 'newCollaboratorIdentifier', null
    ).fail((json) =>
      if json.status == 404
        @set 'errorMessage', "Failed to create new collaborator"
    ).always =>
      @set 'saving', false
      done()

  removeCollaborator: (collaborator) ->
    box = @get 'box'
    collaborators = @get 'collaborators'
    collaborator.remove box.get('id'), (err) ->
      if !err
        collaborators.removeObject collaborator

App.IdeManageCollaboratorsDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/box/ide/manage-collaborators-dialog'
  elementId: 'ide_manage_collaborators_dialog_view'
  contentBinding: 'controller.manageCollaboratorsDialog'
  collaboratorsBinding: 'controller.manageCollaboratorsDialog.collaborators' # controller = ideCollaborationPaneController
  invitesBinding: 'controller.manageCollaboratorsDialog.invites'

  viewDidAppear: ->
    @_super()
    @get('content').fetch()

  viewDidDisappear: ->

  didClickDone: ->
    @get('controller').hideManageCollaboratorsDialog()

  didClickAdd: ->
    @get('content').addCollaborator =>
      @$('.ide_manage_collaborators_dialog_text_field').focus()

  didClickRemove: (collaborator) ->
    @get('content').removeCollaborator collaborator

App.IdeManageCollaboratorsDialogTextField = Em.TextField.extend
  classNames: ['ide_manage_collaborators_dialog_text_field']

  insertNewline: ->
    @get('parentView').didClickAdd()


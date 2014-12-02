App.IdeManageAuthDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/box/ide/manage-auth-dialog'
  elementId: 'ide_manage_auth_dialog_view'
  contentBinding: 'controller.manageAuthDialog' # controller = ideBasicAuthController

  didClickUpdate: ->
    @get('controller').updateAuthentication()

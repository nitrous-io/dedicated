App.IdeManageAuthDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/templates/manage-auth-dialog'
  elementId: 'ide_manage_auth_dialog_view'
  contentBinding: 'controller.manageAuthDialog' # controller = ideBasicAuthController

  didClickUpdate: ->
    @get('controller').updateAuthentication()

App.ConfirmDeleteAccountDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/account/confirm-delete-account-dialog'
  elementId: 'confirm_delete_account_dialog_view'
  contentBinding: 'controller.confirmDeleteAccountDialog' # AccountSettingsIndexController

  didClickProceed: ->
    @get('controller').deleteAccount()
    this.hide()

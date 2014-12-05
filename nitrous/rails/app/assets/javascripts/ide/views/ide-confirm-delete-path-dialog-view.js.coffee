App.IdeConfirmDeletePathDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/templates/confirm-delete-path-dialog'
  elementId: 'confirm_delete_path_dialog_view'
  contentBinding: 'controller.confirmDeletePathDialog' # IdeSidebarController

  didClickDelete: ->
    if path = @get('content.path')
      controller = @get('controller')
      @set 'content', null
      controller.delete path


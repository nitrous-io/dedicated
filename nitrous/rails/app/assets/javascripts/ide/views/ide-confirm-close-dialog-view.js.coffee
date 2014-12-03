App.IdeConfirmCloseDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/box/ide/confirm-close-dialog'
  elementId: 'ide_confirm_close_dialog_view'
  contentBinding: 'controller.confirmCloseDialog' # ideEditorController

  didClickSave: ->
    if tab = @get('content.document')
      controller = @get('controller')
      @set 'content', null
      controller.saveDocument tab

  didClickDontSave: ->
    if tab = @get('content.document')
      @set 'content', null
      @get('controller').removeTab(tab, true)

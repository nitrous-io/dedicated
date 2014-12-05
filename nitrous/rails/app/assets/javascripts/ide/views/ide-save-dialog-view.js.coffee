App.IdeSaveDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/templates/save-dialog'
  elementId: 'ide_save_dialog_view'
  contentBinding: 'controller.saveDialog' # ideEditorController

  viewDidAppear: ->
    @$('#save_path').focus()

  didClickSave: ->
    @get('controller').saveNewDocument()

App.IdeSaveDialogTextField = Em.TextField.extend
  insertNewline: ->
    @get('parentView').didClickSave()


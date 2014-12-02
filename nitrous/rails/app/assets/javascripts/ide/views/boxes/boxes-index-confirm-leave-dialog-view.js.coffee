App.BoxesIndexConfirmLeaveDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/boxes/index-confirm-leave-dialog'
  elementId: 'boxes_confirm_leave_dialog_view'
  contentBinding: 'controller.confirmLeaveDialog' # BoxesIndexController

  didClickLeave: ->
    box = @get('content').get 'box'
    @get('controller').leave box
    this.hide()

App.ChangeCreditCardDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/n2o/credit-cards/change-credit-card-dialog'
  elementId: 'change_credit_card_dialog_view'
  contentBinding: 'controller.changeCreditCardDialog' # AccountSettingsIndexController

  changing: false

  didClickChange: ->
    content = @get 'content'
    @set 'changing', true
    content.updateCard (error) =>
      @set 'changing', false
      unless error
        this.hide()

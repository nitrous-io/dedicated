App.ConfirmCancelSubscriptionDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/account/confirm-cancel-subscription-dialog'
  elementId: 'confirm_cancel_subscription_dialog_view'
  contentBinding: 'controller.confirmCancelSubscriptionDialog' # AccountSettingsIndexController

  didClickProceed: ->
    @get('controller').cancelSubscription()
    this.hide()

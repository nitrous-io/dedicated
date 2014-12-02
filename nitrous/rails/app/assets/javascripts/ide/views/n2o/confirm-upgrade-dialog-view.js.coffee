App.ConfirmUpgradeDialogView = App.IdeModalDialogView.extend
  templateName: 'templates/n2o/confirm-upgrade-dialog'
  elementId: 'confirm_upgrade_dialog_view'
  contentBinding: 'controller.confirmUpgradeDialog' # N2oUpgradeController

  init: ->
    @set 'data-keyboard', 'false'
    @_super.apply this, arguments

  formattedTotal: (->
    if total = @get('content.total')
      "$#{total.toFixed(2)}"
  ).property 'content.total'

  didClickCancel: ->
    @get('controller').upgradeCanceled()
    @_super()

  didClickConfirm: ->
    @get('controller').upgradeConfirmed()
    this.hide()

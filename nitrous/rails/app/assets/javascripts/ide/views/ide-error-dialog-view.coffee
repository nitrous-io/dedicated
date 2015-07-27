App.IdeErrorDialogView = App.IdeModalDialogView.extend
  templateName: 'ide/error-dialog'
  elementId: 'ide-error-dialog-view'
  contentBinding: 'controller.errorDialog'

  actions:
    didClickOK: ->
      # unbind to prevent confirm-close-window popup
      $(window).unbind 'beforeunload.ide'
      window.location.href = @get('content.redirectUrl')
      @hide()

    didClickCancel: ->
      @hide()
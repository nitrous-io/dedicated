App.IdeMaintenanceNoticeView = App.IdeModalDialogView.extend
  templateName: 'ide/templates/maintenance-notice'
  elementId: 'ide_maintenance_notice_view'
  shouldCloseDialog: false

  didInsertElement: ->
    @_super()

  didClickCloseIde: ->
    @set 'shouldCloseDialog', true
    this.hide()

  viewDidDisappear: ->
    if @get 'shouldCloseDialog'
      App.get('router').transitionTo 'boxes.index'


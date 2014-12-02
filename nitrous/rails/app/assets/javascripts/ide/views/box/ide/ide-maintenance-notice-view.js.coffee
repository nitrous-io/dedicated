App.IdeMaintenanceNoticeView = App.IdeModalDialogView.extend
  templateName: 'templates/box/ide/maintenance-notice'
  elementId: 'ide_maintenance_notice_view'
  shouldCloseDialog: false

  didInsertElement: ->
    @_super()
    this.showDialogIfRegionIsDisabled()

  showDialogIfRegionIsDisabled: (->
    content = @get('controller.content')
    if content && !content.get('region.enabled')
      this.show()
  ).observes 'controller.content.region.enabled'

  didClickCloseIde: ->
    @set 'shouldCloseDialog', true
    this.hide()

  viewDidDisappear: ->
    if @get 'shouldCloseDialog'
      App.get('router').transitionTo 'boxes.index'


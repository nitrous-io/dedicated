App.IdeSidebarView = Em.View.extend
  templateName: 'ide/sidebar'
  elementId: 'ide-sidebar-view'

  showContextMenu: (event, path) ->
    return if @get('controller.sftpSocketNotReady') || path.get('loading')
    contextMenuView = @get 'contextMenuView'
    contextMenuView.show event, path

  hideContextMenu: ->
    contextMenuView = @get 'contextMenuView'
    contextMenuView.hide()

  _consoleHeightDidChange: (->
    consoleHeight = App.get 'router.ideLayoutController.consoleHeight'
    @$().css 'paddingBottom', (consoleHeight + 4)
  ).observes 'App.router.ideLayoutController.consoleHeight'

  actions:
    didClickSetHomeAsRoot: ->
      @get('controller').setHomeAsRoot()

    didClickRefresh: ->
      @get('controller').refreshAll()

    didClickUpload: ->
      @get('controller').showUploadDialog()

    didClickShowHidden: ->
      @get('controller').toggleProperty 'hiddenFilesVisible'

    didRightClickBackground: ->
      false

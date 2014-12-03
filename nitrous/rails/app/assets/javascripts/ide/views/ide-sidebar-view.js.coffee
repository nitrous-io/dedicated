App.IdeSidebarView = Em.View.extend
  templateName: 'templates/box/ide/sidebar'
  elementId: 'ide_sidebar_view'

  didClickSetHomeAsRoot: ->
    @get('controller').setHomeAsRoot()

  didClickRefresh: ->
    @get('controller').refreshAll()

  didClickUpload: ->
    @get('controller').showUploadDialog()

  didClickShowHidden: ->
    @get('controller').toggleProperty 'hiddenFilesVisible'

  didRightClickBackground: (event) ->
    event.preventDefault()

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


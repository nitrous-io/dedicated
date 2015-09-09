App.IdeSidebarView = Em.View.extend
  templateName: 'ide/sidebar'
  elementId: 'ide-sidebar-view'
  classNameBindings: ['App.fileTreeHidden:hidden']

  showContextMenu: (event, path) ->
    return if @get('controller.opsSocketNotReady') || path.get('loading')
    contextMenuView = @get 'contextMenuView'
    contextMenuView.show event, path

  hideContextMenu: ->
    contextMenuView = @get 'contextMenuView'
    contextMenuView.hide()

  _fileTreeHiddenDidChange: (->
    $this = @$()
    ml = if App.get 'fileTreeHidden' then -$this.outerWidth() else 0
    $this.css('margin-left': ml)
  ).observes 'App.fileTreeHidden'

  showActiveFilesPane: (->
    if (override = @get 'controller.overrideActiveFilesPane') != null
      return @get('controller.overrideActiveFilesPane')

    return @get('controller.activeFilesMonitor')?.get('sortedFiles').any (file)->
      file.anyRemote
  ).property 'controller.overrideActiveFilesPane', 'controller.activeFilesMonitor.sortedFiles'

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

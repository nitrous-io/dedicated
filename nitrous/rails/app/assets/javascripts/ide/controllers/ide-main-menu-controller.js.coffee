activeDocumentSet = (attr, val) ->
  if tab = App.get 'router.ideEditorController.activeTab'
    tab.set attr, val

App.IdeMainMenuController = Em.Controller.extend
  needs: ['ideEditor', 'ideConsole', 'ideFindInProject', 'ideFileFinder', 'ideGotoLine', 'ideSidebar']
  initialize: ->
  reset: ->

  fileNew: ->
    if editorController = @get 'controllers.ideEditor'
      editorController.createTab()

  fileSave: ->
    if editorController = @get 'controllers.ideEditor'
      editorController.saveDocument editorController.get('activeTab')

  fileGoToFile: ->
    if controller = @get 'controllers.ideFileFinder'
      controller.showFindDialog()

  fileClose: ->
    if editorController = @get 'controllers.ideEditor'
      editorController.removeTab()

  fileRefreshTree: ->
    if sidebarController = App.get 'router.ideSidebarController'
      sidebarController.refreshAll()

  findFind: ->
    activeDocumentSet 'findTrigger', true

  findFindNext: ->
    activeDocumentSet 'findNextTrigger', true

  findSelectNext: ->
    activeDocumentSet 'selectNextTrigger', true

  findFindPrevious: ->
    activeDocumentSet 'findPrevTrigger', true

  findFindInFiles: ->
    if controller = @get 'controllers.ideFindInProject'
      path = this.get('controllers.ideSidebar.rootPath')?.absolutePath
      controller.showFindDialog(path)

  findReplace: ->
    activeDocumentSet 'replaceTrigger', true

  findReplaceAll: ->
    activeDocumentSet 'replaceAllTrigger', true

  editGoToLine: ->
    if controller = @get 'controllers.ideGotoLine'
      controller.showGoToLineDialog()

  editFoldCode: ->
    activeDocumentSet 'foldCodeTrigger', true

  editUnfoldCode: ->
    activeDocumentSet 'unfoldCodeTrigger', true

  editToggleComment: ->
    activeDocumentSet 'toggleCommentTrigger', true

  editSetKeybindings: (type) ->
    if editorController = @get 'controllers.ideEditor'
      editorController.set 'keybindings', type

  editSplitSelectionIntoLines: (type) ->
    activeDocumentSet 'splitSelectionIntoLinesTrigger', true

  windowFocusConsole: ->
    if doc = @get 'controllers.ideEditor.activeTab'
      doc.set 'focused', false
    if consoleSession = @get 'controllers.ideConsole.activeTab'
      consoleSession.set 'focused', true

  windowFocusEditor: ->
    if doc = @get 'controllers.ideEditor.activeTab'
      doc.set 'focused', true
    if consoleSession = @get 'controllers.ideConsole.activeTab'
      consoleSession.set 'focused', false

  windowSelectNextTab: ->
    if editorController = @get 'controllers.ideEditor'
      editorController.makeNextTabActive()

  windowSelectPreviousTab: ->
    if editorController = @get 'controllers.ideEditor'
      editorController.makePreviousTabActive()

  windowMinimizeConsole: ->
    App.set 'consoleMinimized', true
    App.set 'consoleFullscreen', false

  windowRestoreConsole: ->
    App.set 'consoleMinimized', false
    @windowFocusConsole()

  windowToggleFullScreenConsole: ->
    App.toggleProperty 'consoleFullscreen'
    App.set('consoleMinimized', false) if App.get('consoleFullscreen')
    window.setTimeout(=>
      @windowFocusConsole()
    , 50)

  viewSplitEditorPaneVertically: ->
    App.get('router.ideEditorController').splitActivePaneVertically()

  viewSplitEditorPaneHorizontally: ->
    App.get('router.ideEditorController').splitActivePaneHorizontally()

  viewCloseEditorPane: ->
    App.get('router.ideEditorController').closeActivePane()

  windowSelectTheme: (theme) ->
    if editorController = @get 'controllers.ideEditor'
      editorController.set 'theme', theme

  viewToggleLineNumbers: ->
    @_toggleEditorProperty 'showLineNumbers'

  viewSetRulerColumn: (column) ->
    if editorController = @get 'controllers.ideEditor'
      editorController.set 'rulerColumn', column

  helpNitrousReadme: ->
    if sidebarController = App.get 'router.ideSidebarController'
      sidebarController.openRepoReadme()

  viewToggleTrailingSpace: ->
    @_toggleEditorProperty 'showTrailingSpace'

  viewToggleWrapLines: ->
    @_toggleEditorProperty 'wrapLines'

  viewToggleHighlightLine: ->
    @_toggleEditorProperty 'highlightLine'

  viewToggleMatchBrackets: ->
    @_toggleEditorProperty 'matchBrackets'

  toggleAutoCloseBrackets: ->
    @_toggleEditorProperty 'autoCloseBrackets'

  _toggleEditorProperty: (property) ->
    if editorController = @get 'controllers.ideEditor'
      editorController.toggleProperty(property)

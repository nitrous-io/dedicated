activeDocumentSet = (attr, val) ->
  if tab = App.get 'router.ideEditorController.activeTab'
    tab.set attr, val

App.IdeMainMenuController = Em.Controller.extend
  needs: ['ideEditor', 'ideConsole', 'ideFindInProject', 'ideFileFinder']
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
      App.util.mixpanelIncrement 'ide_opened_file_finder_dialog_count'

  fileClose: ->
    if editorController = @get 'controllers.ideEditor'
      editorController.removeTab()

  fileRefreshTree: ->
    if sidebarController = App.get 'router.ideSidebarController'
      sidebarController.refreshAll()

  editFind: ->
    activeDocumentSet 'findTrigger', true

  editFindNext: ->
    activeDocumentSet 'findNextTrigger', true

  editSelectNext: ->
    activeDocumentSet 'selectNextTrigger', true

  editFindPrevious: ->
    activeDocumentSet 'findPrevTrigger', true

  editFindInProject: ->
    if controller = @get 'controllers.ideFindInProject'
      controller.showFindDialog()
      App.util.mixpanelIncrement 'ide_opened_find_in_project_dialog_count'

  editReplace: ->
    activeDocumentSet 'replaceTrigger', true

  editReplaceAll: ->
    activeDocumentSet 'replaceAllTrigger', true

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

  viewFocusConsole: ->
    if doc = @get 'controllers.ideEditor.activeTab'
      doc.set 'focused', false
    if consoleSession = @get 'controllers.ideConsole.activeTab'
      consoleSession.set 'focused', true

    App.util.mixpanelIncrement 'ide_focused_console_count'

  viewFocusEditor: ->
    if doc = @get 'controllers.ideEditor.activeTab'
      doc.set 'focused', true
    if consoleSession = @get 'controllers.ideConsole.activeTab'
      consoleSession.set 'focused', false

  viewSelectNextTab: ->
    if editorController = @get 'controllers.ideEditor'
      editorController.makeNextTabActive()

  viewSelectPreviousTab: ->
    if editorController = @get 'controllers.ideEditor'
      editorController.makePreviousTabActive()

  viewToggleFullScreenConsole: ->
    if consoleController = @get 'controllers.ideConsole'
      consoleController.toggleProperty 'fullscreen'
      App.util.mixpanelIncrement 'ide_toggled_console_fullscreen_count'
    this.viewFocusConsole()

  viewToggleFullScreenEditor: ->
    @_toggleEditorProperty 'fullscreen'
    this.viewFocusEditor()

  viewSelectTheme: (theme) ->
    if editorController = @get 'controllers.ideEditor'
      editorController.set 'theme', theme

  viewToggleLineNumbers: ->
    @_toggleEditorProperty 'showLineNumbers'

  viewSetRulerColumn: (column) ->
    if editorController = @get 'controllers.ideEditor'
      editorController.set 'rulerColumn', column

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

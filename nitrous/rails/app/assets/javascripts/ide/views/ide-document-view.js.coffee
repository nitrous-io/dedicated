TAB_SIZES = [2, 3, 4, 8]

App.IdeDocumentView = Em.View.extend
  templateName: 'templates/box/ide/document'
  classNames: ['ide_document_view']

  document: null
  editor: null
  firepad: null
  firebase: null
  collabMode: false
  firepadEnabling: false

  mode: 'text/plain'
  softTabsBinding: 'controller.softTabs'
  tabSizeBinding: 'controller.tabSize'
  themeBinding: 'controller.theme'
  wrapLinesBinding: 'controller.wrapLines'
  highlightLineBinding: 'controller.highlightLine'
  showLineNumbersBinding: 'controller.showLineNumbers'
  rulerColumnBinding: 'controller.rulerColumn'
  showTrailingSpaceBinding: 'controller.showTrailingSpace'
  matchBracketsBinding: 'controller.matchBrackets'
  autoCloseBracketsBinding: 'controller.autoCloseBrackets'
  keybindingsBinding: 'controller.keybindings'

  foldGutterClass: 'CodeMirror-foldgutter'

  gotoLine: null # changes to this trigger line movement in CodeMirror

  init: ->
    this.documentPathDidChange()
    @_super.apply this, arguments

  didInsertElement: ->
    @_super()
    this.insertEditor()

  willDestroyElement: ->
    @_super()
    @set 'collabMode', false
    this.destroyEditor()

  modeName: (->
    CodeMirror.MODES[@get('mode')]
  ).property 'mode'

  collabModeDisabled: (->
    return false if @get('collabMode')
    !@get('document.path.absolutePath') || !@get('document.clean') || @get('firepadEnabling')
  ).property 'collabMode', 'document.path.absolutePath', 'document.clean', 'firepadEnabling'

  availableModes: (->
    mode = @get 'mode'
    _.map CodeMirror.MODES_ARRAY, (m) ->
      { name: m.name, mode: m.mode, active: m.mode == mode }
  ).property 'mode'

  availableTabSizes: (->
    tabSize = @get 'tabSize'
    _.map TAB_SIZES, (size) ->
      { size: size, active: size == tabSize }
  ).property 'tabSize'

  saving: (->
    if loadCount = @get 'document.path.loadCount'
      loadCount > 0
  ).property 'document.path.loadCount'

  savingDisabled: (->
    @get('saving') || App.get('offline')
  ).property 'saving', 'App.offline'

  triggerFind: (->
    if @get('document.findTrigger') && (editor = @get 'editor')
      editor.find()
      @set 'document.findTrigger', false
  ).observes 'document.findTrigger'

  triggerFindNext: (->
    if @get('document.findNextTrigger') && (editor = @get 'editor')
      editor.findNext()
      @set 'document.findNextTrigger', false
  ).observes 'document.findNextTrigger'

  triggerSelectNext: (->
    if @get('document.selectNextTrigger') && (editor = @get 'editor')
      editor.selectNext()
      @set 'document.selectNextTrigger', false
  ).observes 'document.selectNextTrigger'

  triggerFindPrev: (->
    if @get('document.findPrevTrigger') && (editor = @get 'editor')
      editor.findPrev()
      @set 'document.findPrevTrigger', false
  ).observes 'document.findPrevTrigger'

  triggerReplace: (->
    if @get('document.replaceTrigger') && (editor = @get 'editor')
      editor.replace()
      @set 'document.replaceTrigger', false
  ).observes 'document.replaceTrigger'

  triggerReplaceAll: (->
    if @get('document.replaceAllTrigger') && (editor = @get 'editor')
      editor.replaceAll()
      @set 'document.replaceAllTrigger', false
  ).observes 'document.replaceAllTrigger'

  triggerFoldCode: (->
    if @get('document.foldCodeTrigger') && (editor = @get 'editor')
      editor.fold()
      @set 'document.foldCodeTrigger', false
  ).observes 'document.foldCodeTrigger'

  triggerUnfoldCode: (->
    if @get('document.unfoldCodeTrigger') && (editor = @get 'editor')
      editor.unfold()
      @set 'document.unfoldCodeTrigger', false
  ).observes 'document.unfoldCodeTrigger'

  triggerToggleComment: (->
    if @get('document.toggleCommentTrigger') && (editor = @get 'editor')
      editor.toggleComment()
      @set 'document.toggleCommentTrigger', false
  ).observes 'document.toggleCommentTrigger'

  closeDocument: (->
    if @get 'document.path.deleted'
      this.remove()
  ).observes 'document.path.deleted'

  jumpToLine: (->
    if line = @get 'document.gotoLine'
      if editor = @get('editor')
        editor.gotoLine(line)
  ).observes 'document.gotoLine'

  splitSelectionIntoLines: (->
    if editor = @get('editor')
      editor.splitSelectionIntoLines()
    @set 'document.splitSelectionIntoLinesTrigger', false
  ).observes 'document.splitSelectionIntoLinesTrigger'

  insertEditor: ->
    $codeEditor = @$('.code_editor')
    return if $codeEditor.length == 0

    settings = @getProperties(App.Editor.SETTINGS)
    settings.domTarget = $codeEditor[0]
    settings.mode = @get 'mode'
    settings.text = @get 'document.content'
    editor = App.Editor.create(settings)

    editor.on 'change', =>
      unless @isDestroyed
        if @get('document.content') != (content = editor.get('text'))
          @set 'document.content', content
        if @get('document.clean') != (clean = editor.isClean())
          @set 'document.clean', clean

    editor.on 'focus', =>
      if !@get 'document.focused'
        @set 'document.focused', true

    editor.on 'blur', =>
      if @get 'document.focused'
        @set 'document.focused', false

    @set 'editor', editor

  destroyEditor: ->
    if editor = @get 'editor'
      editor.destroy()
      @set 'editor', null

  updateEditor: (setting) ->
    if editor = @get 'editor'
      editor.set setting, @get(setting)

  modeDidChange: (->
    @updateEditor('mode')
  ).observes 'mode'

  tabSizeDidChange: (->
    @updateEditor('tabSize')
  ).observes 'tabSize'

  softTabsDidChange: (->
    @updateEditor('softTabs')
  ).observes 'softTabs'

  themeDidChange: (->
    @updateEditor('theme')
  ).observes 'theme'

  wrapLinesDidChange: (->
    @updateEditor('wrapLines')
  ).observes 'wrapLines'

  highlightLineDidChange: (->
    @updateEditor('highlightLine')
  ).observes 'highlightLine'

  showLineNumbersDidChange: (->
    @updateEditor('showLineNumbers')
  ).observes 'showLineNumbers'

  rulerColumnDidChange: (->
    @updateEditor('rulerColumn')
  ).observes 'rulerColumn'

  showTrailingSpaceDidChange: (->
    @updateEditor('showTrailingSpace')
  ).observes 'showTrailingSpace'

  matchBracketsDidChange: (->
    @updateEditor('matchBrackets')
  ).observes 'matchBrackets'

  autoCloseBracketsDidChange: (->
    @updateEditor('autoCloseBrackets')
  ).observes 'autoCloseBrackets'

  keybindingsDidChange: (->
    @updateEditor('keybindings')
  ).observes 'keybindings'

  contentDidChange: (->
    if editor = @get 'editor'
      editor.set 'text', @get 'document.content'
  ).observes 'document.content'

  cleanDidChange: (->
    if editor = @get 'editor'
      documentIsClean = @get('document.clean')

      if editor.isClean() != documentIsClean
        editor.markClean()
      if documentIsClean && @get('collabMode')
        this._setCollabSavedState()
  ).observes 'document.clean'

  consoleHeightDidChange: (->
    if editor = @get 'editor'
      editor.refresh()
  ).observes 'App.router.ideLayoutController.consoleHeight'

  editorWidthDidChange: (->
    if editor = @get 'editor'
      editor.refresh()
  ).observes(
    'App.router.ideLayoutController.sidebarWidth',
    'App.router.ideLayoutController.collaborationPaneWidth',
    'App.router.ideLayoutController.collaborationPaneHidden'
  )

  editorFullscreenToggled: (->
    if editor = @get 'editor'
      editor.refresh()
  ).observes 'App.router.ideEditorController.fullscreen'

  focusedDidChange: (->
    if editor = @get 'editor'
      if focused = @get 'document.focused'
        editor.refresh()
        editor.focus()
      #else
        #@$('textarea')[0].blur()
  ).observes 'document.focused'

  documentPathDidChange: (->
    if fileName = @get 'document.path.name'
      @set 'mode', App.FileTypeDetect.detect fileName
  ).observes 'document.path.name'

  collabModeDidChange: (->
    if @get 'collabMode'
      this._enableFirepad()
    else
      this._destroyFirepad()
  ).observes 'collabMode'

  didClickSave: ->
    @get('controller').saveDocument @get('document')

  didClickFind: ->
    if document = @get('document')
      document.set 'findTrigger', true

  didSelectMode: (selectedMode) ->
    @set 'mode', selectedMode

  didSelectTabSize: (selectedTabSize) ->
    @set 'tabSize', selectedTabSize

  didClickSoftTabToggle: ->
    @set 'softTabs', !@get('softTabs')

  didClickCollabToggle: ->
    this.toggleProperty 'collabMode'

  _generateFirepadURL: ->
    firebaseHost = App.get 'config.firebaseHost'
    boxId = App.get 'router.boxIdeController.content.id'
    documentPath = @get 'document.path'

    if documentPath && (absoluteDocumentPath = documentPath.get('absolutePath'))
      pathToken = window.btoa window.unescape(window.encodeURIComponent(absoluteDocumentPath)) # escape unicode and then base64ize it
      "#{firebaseHost}/box/#{boxId}/pad/#{pathToken}"

  _enableFirepad: ->
    editor = @get 'editor'
    cm = editor.get 'codeMirror'
    documentPath = @get 'document.path'
    firepadURL = this._generateFirepadURL documentPath
    firebaseAuthenticated = App.get 'router.ideCollaborationPaneController.firebaseAuthenticated'

    @set 'firepadEnabling', true

    if firebaseAuthenticated && firepadURL
      firebase = new Firebase firepadURL
      @set 'firebase', firebase

      # check if there are any active users for the pad
      # if not start it as a new session
      this._hasActivePadUsers firebase, (usersPresent) =>
        currentUserName = App.get 'currentUser.username'
        currentUserColor = App.get 'router.ideCollaborationPaneController.userColor'
        content = @get 'document.content'

        cm.setValue ''
        cm.setOption 'readOnly', 'nocursor'

        firepad = Firepad.fromCodeMirror firebase, cm, userId: currentUserName, userColor: currentUserColor
        @set 'firepad', firepad

        firepad.on 'ready', =>
          if firepad.isHistoryEmpty() || !usersPresent
            firepad.setText content
          cm.setOption 'readOnly', false
          cm.refresh()
          this._resetDirtyBitIfContentNotModified content
          this._syncSavedState firebase
          this._notifyCollabEvent()
          @set 'collabMode', true
          @set 'firepadEnabling', false
          App.util.mixpanel 'IDE Collab Mode'
          App.util.mixpanelIncrement 'ide_collab_mode_count'

  _destroyFirepad: ->
    if firepad = @get 'firepad'
      firepad.dispose()
    @set 'firepad', null

  _hasActivePadUsers: (firebase, callback)->
    firebase.child('users').once 'value', (usersSnapshot) ->
      callback usersSnapshot.hasChildren()

  _syncSavedState: (firebase) ->
    document = @get 'document'
    firebase.child('saved').on 'value', (savedSnapshot) ->
      saved = savedSnapshot.val()
      if saved
        document.set 'clean', true
        firebase.child('saved').set null

  _setCollabSavedState: ->
    if firebase = @get 'firebase'
      firebase.child('saved').set true

  _resetDirtyBitIfContentNotModified: (originalContent) ->
    editor = @get 'editor'
    if editor.get('text') == originalContent
      editor.markClean()
      @set 'document.clean', true

  _notifyCollabEvent: ->
    path = @get 'document.path'
    collaborationPaneController = App.get 'router.ideCollaborationPaneController'
    if path && collaborationPaneController
      if absolutePath = path.get 'absolutePath'
        collaborationPaneController.sendMessage absolutePath, App.CollabMessage.TypeCollabMode

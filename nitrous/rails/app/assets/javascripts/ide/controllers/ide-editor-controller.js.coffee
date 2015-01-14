App.IdeEditorController = Em.ArrayController.extend App.IdeTabControllerMixin,
  modelClass: App.Document
  content: null
  saveDialog: null
  confirmCloseDialog: null
  fullscreenBinding: 'App.editorFullscreen'
  chatNotification: null

  opsSocketBinding: Em.Binding.oneWay 'App.router.ideController.opsSocket'
  opsSocketReadyBinding: Em.Binding.oneWay 'App.router.ideController.opsSocketReady'

  softTabs: true
  tabSize: 2
  theme: 'solarized dark'
  wrapLines: true
  highlightLine: true
  showLineNumbers: true
  rulerColumn: null
  showTrailingSpace: false
  matchBrackets: true
  autoCloseBrackets: true
  keybindings: 'default'

  draggedTab: null # tab being dragged
  droppableIndex: null # index where the currently dragged tab would be inserted

  initialize: ->
    @set 'content', []
    @set 'fullscreen', false

    @useUserEditorSettings()
    @observeEditorSettings()

  reset: ->
    @set 'content', null
    @set 'saveDialog', null
    @set 'confirmCloseDialog', null
    @set 'fullscreen', false

  useUserEditorSettings: ->
    userSettings = App.get('currentUser.user_settings') || {}
    editorSettings = {}
    _.each App.Editor.SETTINGS, (k) ->
      editorSettings[k] = userSettings[k.underscore()]

    # Backwards compatibility: theme and tab settings used to be stored in HTML
    # local storage at the box level.
    if oldSettings = @localStorageEditorSettings()
      editorSettings.softTabs ?= oldSettings.softTabs
      editorSettings.tabSize ?= oldSettings.tabSize
      editorSettings.theme ?= oldSettings.theme

    _.each App.Editor.SETTINGS, (k) =>
      @set(k, editorSettings[k]) if editorSettings[k]?

  localStorageEditorSettings: ->
    if boxId = App.get 'router.ideController.content.id'
      App.localStorage.delete("box:#{boxId}:editorSettings") || App.localStorage.delete("box:#{boxId}:lastTabSettings")

  observeEditorSettings: ->
    _.each App.Editor.SETTINGS, (setting) =>
      @addObserver(setting, this, 'updateUserSettings')

  updateUserSettings: ->
    currentUser = App.get('currentUser')
    userSettings = currentUser.get('user_settings') || {}
    _.each App.Editor.SETTINGS, (k) =>
      userSettings[k.underscore()] = @get(k)
    # FIXME Enable user settings when backend supports it
    # currentUser.updateSettings()

  createTab: (attrs={}, modelClass=null) ->
    App.util.mixpanel 'IDE New Document Tab'
    App.util.mixpanelIncrement 'ide_new_document_tab_count'
    tab = @_super(attrs, modelClass)
    tab

  removeTab: (tab, forceRemove = false) ->
    tab = @get 'activeTab' unless tab
    if tab.get('clean') || forceRemove
      @_super(tab)
    else
      @set 'confirmCloseDialog', App.ConfirmCloseDialog.create document: tab

  saveDocument: (document) ->
    return unless @get 'opsSocketReady'
    return if document.get('isImageFile')

    if path = document.get 'path'
      if opsSocket = @get 'opsSocket'
        path.incrementProperty 'loadCount'
        opsSocket.writeFile path.get('absolutePath'), document.get('content'), (err, absolutePath) ->
          path.decrementProperty 'loadCount'
          if err
            App.set 'flashMessage', { type: 'alert-error', text: "Error saving file '#{path.get('absolutePath')}'" }
            App.util.mixpanel 'IDE Document Save Error'
            App.util.mixpanelAppendUser "SaveError": err
            return
          document.set 'clean', true
          App.util.mixpanel 'IDE Document Saved'
          App.util.mixpanelIncrement 'ide_document_saved_count'
    else
      @set 'saveDialog', App.SaveDialog.create document: document

  saveNewDocument: ->
    return unless @get 'opsSocketReady'
    saveDialog = @get 'saveDialog'
    unless saveDialog.validate()
      saveDialog.set 'saving', true
      documentContent = saveDialog.get 'document.content'
      # FIXME probably not the best way to scope to home path
      path = saveDialog.get 'path'
      homePath = App.get('container.homePath')
      absolutePath = "#{homePath}/#{path}"
      if opsSocket = @get 'opsSocket'
        opsSocket.writeNewFile absolutePath, documentContent, _.bind(@didWriteNewFileContent, this)

  didWriteNewFileContent: (err, absolutePath) ->
    saveDialog = @get 'saveDialog'
    if err
      # FIXME current errors returned by freighter is cryptic.
      # showing a generic message for now
      errors = {base: ["Error occurred while trying to save the file."]}
      saveDialog.set 'errors', errors
    else
      path = App.Path.create absolutePath: absolutePath
      saveDialog.set 'document.path', path
      saveDialog.set 'document.clean', true
      saveDialog.set 'document.focused', true
      sidebarController = App.get 'router.ideSidebarController'
      sidebarController.refreshPath path.get('parentAbsolutePath')
      @set 'saveDialog.hide', true
      App.util.mixpanel 'IDE New Document Saved'
      App.util.mixpanelIncrement 'ide_new_document_saved_count'
    saveDialog.set 'saving', false

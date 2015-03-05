App.IdeEditorController = Em.ArrayController.extend App.IdeTabControllerMixin,
  modelClass: App.Document
  content: null
  rootPane: null
  activePane: Em.computed.alias 'content'
  saveDialog: null
  confirmCloseDialog: null
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

  initialize: ->
    editorPane = App.EditorPane.create(tabs: Em.A())
    @set 'rootPane', editorPane
    @set 'activePane', editorPane

    @useUserSettings()

  reset: ->
    @set 'content', null
    @set 'saveDialog', null
    @set 'confirmCloseDialog', null

  useUserSettings: ->
    userSettings = App.get('userSettings') || {}
    _.each App.Editor.SETTINGS, (k) =>
      setting = userSettings[k.underscore()]
      if setting?
        @set(k, setting)
        @addObserver(k, this, 'updateUserSettings')

  updateUserSettings: ->
    userSettings = App.get('userSettings')
    if userSettings
      _.each App.Editor.SETTINGS, (k) =>
        userSettings[k.underscore()] = @get(k)
      userSettings.save()

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
            return
          document.set 'clean', true
          Mp.track Mp.EventIDEUserSavedFile
          Mp.increment Mp.PropertyIDEFileSaveCount, new_file: 'false'
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
      # TODO: Freighter should be returning a proper error code for file exists.
      # Revisit after #381 is merged.
      errors = if err.match /file exists/
        {base: ["already exists. Please enter a different filename."]}
      else
        {base: ["Error occurred while trying to save the file."]}
      saveDialog.set 'errors', errors
    else
      path = App.Path.create absolutePath: absolutePath
      saveDialog.set 'document.path', path
      saveDialog.set 'document.clean', true
      saveDialog.set 'document.focused', true
      sidebarController = App.get 'router.ideSidebarController'
      sidebarController.refreshPath path.get('parentAbsolutePath')
      @set 'saveDialog.hide', true
      Mp.track Mp.EventIDEUserSavedFile, new_file: 'true'
      Mp.increment Mp.PropertyIDEFileSaveCount
    saveDialog.set 'saving', false

  addTab: ->
    @_super.apply this, arguments
    Mp.track Mp.EventIDEUserOpenedFile

  createTab: (attrs={}, modelClass=null) ->
    rootPane = @get 'rootPane'
    if rootPane.findBy('path', attrs.path)
      return # disallow duplicate tabs for now
    @_super(attrs, modelClass)

  splitActivePaneVertically: ->
    pane = @get 'activePane'
    pane.split()
    @set 'activePane', pane.get('left')

  splitActivePaneHorizontally: ->
    pane = @get 'activePane'
    pane.split(true)
    @set 'activePane', pane.get('left')

  closeActivePane: ->
    pane = @get 'activePane'
    pane.close()

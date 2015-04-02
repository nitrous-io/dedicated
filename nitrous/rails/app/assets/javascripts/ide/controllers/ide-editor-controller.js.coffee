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

    # We do this so that saving using :w in Vim mode works.
    # Sucks that this is a CodeMirror "global".
    CodeMirror.commands.save = (cm) =>
      @saveDocument(@get('activeTab'))

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

  saveGoDocument: (document) ->
    path = document.get 'path'
    opsSocket = @get 'opsSocket'
    if !path || !opsSocket
      return
    cmd = ["goimports", "-w", path.get('absolutePath')]
    cwd = ''
    env = []
    opsSocket.run cmd, cwd, env, (err, cmdErr, stdout, stderr) =>
      if err || cmdErr
        document.set 'clean', true
        return
      sidebarController = App.get 'router.ideSidebarController'
      if sidebarController
        sidebarController.refreshTextFileContent path

  saveDocument: (document) ->
    return unless @get 'opsSocketReady'
    return if document.get('isImageFile')

    if path = document.get 'path'
      if opsSocket = @get 'opsSocket'
        path.incrementProperty 'loadCount'
        opsSocket.writeFile path.get('absolutePath'), document.get('content'), (err, absolutePath) =>
          path.decrementProperty 'loadCount'
          if err
            App.set 'flashMessage', { type: 'alert-error', text: "Error saving file '#{path.get('absolutePath')}'" }
            return
          Mp.track Mp.EventIDEUserSavedFile
          Mp.increment Mp.PropertyIDEFileSaveCount, new_file: 'false'

          ideController = App.get 'router.ideController'
          hasCmdRun = ideController?.freighterSupports("hasCmdRun")
          if hasCmdRun && path.get('absolutePath').match(/\.go$/)
            @saveGoDocument(document)
          else
            document.set 'clean', true
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

  # omit `path` or provide `multiple=true` in attrs to allow for multiple tabs
  # with same path
  createTab: (attrs={}, modelClass=null) ->
    rootPane = @get 'rootPane'
    if attrs.multiple != true && rootPane.findBy('path', attrs.path)
      return # disallow duplicate tabs for now, unless specified by caller
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

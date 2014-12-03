App.IdeSidebarController = Em.ArrayController.extend
  needs: ['ideFindInProject']

  homePath: null # home (~/) path
  rootPath: null # current root path in the sidebar
  content:  null # array of all path objects
  uploadDialog: null
  confirmDeletePathDialog: null

  sftpSocketBinding: Em.Binding.oneWay 'App.router.boxIdeController.sftpSocket'
  sftpSocketReadyBinding: Em.Binding.oneWay 'sftpSocket.started'
  sftpSocketNotReady: (->
    !@get 'sftpSocketReady'
  ).property 'sftpSocketReady'

  repositoryBinding: Em.Binding.oneWay 'App.router.boxIdeController.repository'

  hiddenFilesVisible: false

  currentlySelectedPath: null

  # attributes used for drag&drop
  currentlyDraggedPath: null
  currentDroppable: null

  initialize: ->
    @set 'homePath', homePath = App.Path.create absolutePath: '', directory: true
    @set 'rootPath', homePath
    @set 'content', Em.A([homePath])

    @set 'readmePath', readmePath = App.Path.create absolutePath: 'README.md', directory: false

  reset: ->
    @set 'homePath', null
    @set 'rootPath', null
    @set 'content', null
    @set 'uploadDialog', null
    @set 'currentlyDraggedPath', null
    @set 'currentDroppable', null

  sftpSocketDidStartForTheFirstTime: (->
    if @get 'sftpSocket.startedFirstTime'
      Em.run.next =>
        this.openPath @get('rootPath')
        if repo = @get('repository')
          this.openRepoDir()
          this.openRepoReadme()
        else
          # Open README.md, if it's not present, then don't show an error
          if boxId = App.get 'router.boxIdeController.content.id'
            cacheKey = "box:#{boxId}:shownReadme"
            if !App.localStorage.get(cacheKey)
              this.openPath @get('readmePath'), error: false
              App.localStorage.set(cacheKey, true)
  ).observes 'sftpSocket.startedFirstTime'

  openRepoDir: ->
    workspacePath = App.Path.create absolutePath: 'workspace', directory: true
    this.openPath workspacePath, error: false
    repoPath = App.Path.create absolutePath: "workspace/#{@get('repository.shortName')}", directory: true
    this.openPath repoPath, error: false

  openRepoReadme: ->
    if fileToOpenPath = @get('repository.file_to_open')
      fileToOpenPath = App.Path.create absolutePath: "workspace/#{@get('repository.shortName')}/#{fileToOpenPath}"
      this.openPath fileToOpenPath, error: false
    else
      _.each ['README.md', 'README.markdown', 'README'], (readme) =>
        readmePath = App.Path.create absolutePath: "workspace/#{@get('repository.shortName')}/#{readme}"
        this.openPath readmePath, error: false

  # settings options:
  #   done: callback (document)
  #   error: callback (errorMessage)
  openPath: (path, settings = {}) ->
    if settings.error != false
      settings.error = (message) -> # default error handler
        App.set 'flashMessage', { type: 'error', text: 'Cannot open the selected file.' }
    if !path.get('loading') && @get('sftpSocketReady')
      if path.get 'directory'
        path.set 'expanded', !path.get('expanded')
        if path.get 'expanded'
          this.fetchFileList path
      else
        editorController = App.get 'router.ideEditorController'
        if document = editorController.findProperty 'path', path
          editorController.makeTabActive document
          if settings.done
            settings.done(document)
        else
          this.fetchFileContent path, settings
        if settings.source
          App.util.mixpanel 'Opened File Tab', source: settings.source


  downloadPath: (path) ->
    if !path.get('loading') && @get('sftpSocketReady')
      if path.get 'file'
        sftpSocket = @get 'sftpSocket'

        sftpSocket.downloadFile path.get('absolutePath'), (err, path) ->
          if err
            App.set 'flashMessage', { type: 'error', text: "Cannot download the selected file." }
            return

          boxServerURI = App.get 'router.boxIdeController.content.box_server_uri'
          boxId = App.get 'router.boxIdeController.content.id'

          if boxServerURI && boxId
            App.authToken.obtain 'rainbow', (err, authToken) ->
              return if err
              downloadFrame = $('<iframe></iframe>')
              downloadFrame.css 'display', 'none'
              downloadFrame.attr 'src', "#{boxServerURI}/download?id=#{boxId}&path=#{path}&auth_token=#{authToken}"
              $('body').append downloadFrame
              window.setTimeout(->
                downloadFrame.remove()
              , 5000)

  fetchFileList: (path) ->
    sftpSocket = @get 'sftpSocket'
    path.incrementProperty 'loadCount'
    sftpSocket.fetchFileList path.get('absolutePath'), (err, absolutePath, content) ->
      path.decrementProperty 'loadCount'
      unless err
        path.updateContent content

  fetchFileContent: (path, settings = {}) ->
    sftpSocket = @get 'sftpSocket'
    path.incrementProperty 'loadCount'
    sftpSocket.fetchFileContent path.get('absolutePath'), (err, absolutePath, contentType, content) ->
      path.decrementProperty 'loadCount'
      if err
        if settings.error
          settings.error("Cannot open the selected file")
        return

      editorController = App.get 'router.ideEditorController'
      tabClass = if contentType?.match /^image/ then App.Image else App.Document
      tab = editorController.createTab(
        content: content
        path: path
        contentType: contentType
      , tabClass)
      if settings.done
        settings.done(tab)

  refresh: (path) ->
    if path.get('directory') && path.get('expanded')
      delay = 0

      for subpath in path.get 'arrangedContent'
        if subpath.get('directory') && subpath.get('expanded')
          delay += @constructor.REFRESH_DELAY
          ((controller, subpath, delay) ->
            window.setTimeout ->
              controller.refresh subpath
            , delay
          )(this, subpath, delay)

      unless path.get 'loading'
        this.fetchFileList path

  refreshPath: (pathString) ->
    homePath = @get 'homePath'
    if path = homePath.findByRelativePath pathString
      this.refresh path

  refreshAll: ->
    this.refresh @get('rootPath')

  findInFolder: (path) ->
    controller = @get 'controllers.ideFindInProject'
    if controller
      controller.showFindDialog(path.absolutePath)

  setHomeAsRoot: ->
    @set 'rootPath', @get('homePath')

  setPathAsRoot: (path) ->
    @set 'rootPath', path

  newPath: (path, directory) ->
    this.openPath path if !path.get 'expanded'
    newPath = App.Path.create
      absolutePath: "#{path.get('absolutePath')}/__aio_untitled_path__"
      new: true
      editable: true
      directory: directory
    path.addObject newPath

  saveNewPath: (path, name) ->
    homePath = @get 'homePath'
    parentAbsolutePath = path.get 'parentAbsolutePath'
    if parentPath = homePath.findByRelativePath parentAbsolutePath
      sftpSocket = @get 'sftpSocket'
      absolutePath = _.chain([parentAbsolutePath, name]).compact().join('/').value()

      callback = =>
        path.set 'new', false # temp paths with 'new: false' get removed on refresh
        this.refresh parentPath

      if path.get 'directory'
        sftpSocket.makeDirectory absolutePath, callback
      else
        sftpSocket.writeNewFile absolutePath, '', callback

  cancelNewPath: (path) ->
    homePath = @get 'homePath'
    if parentPath = homePath.findByRelativePath path.get('parentAbsolutePath')
      parentPath.get('content').removeObject path

  renamePath: (path) ->
    path.set 'editable', true

  # apply rename operations to path
  # - path: the path object that we want to rename
  # - newPath: the path object / new name of the path.
  # - isDrag: if true, newPath will be the new path object that we drag the
  # path into a.k.a new parent path
  applyRenamedPath: (path, newPath, isDrag) ->
    if typeof newPath == "string"
      return this.resetRenamedPath path if newPath == path.get 'name'
    if isDrag && !(newPath instanceof App.Path)
      return console.warning("expected a App.Path object")
    if !isDrag && !(typeof newPath == "string")
      return console.warning("expected a path string")

    homePath = @get('homePath')
    parentAbsolutePath = path.get 'parentAbsolutePath'
    # current directory that hold the path
    parentPath = homePath.findByRelativePath parentAbsolutePath
    return unless parentPath

    sftpSocket = @get 'sftpSocket'
    currentPathName = path.get 'name'

    if isDrag
      # if this is a drag operation, newPath is going to be the path that we drag
      # the file into, while the name of the file will be kept
      newParentPath = newPath.get('absolutePath')
      newPathName = currentPathName
    else
      # otherwise, this is the rename operation, which the parentPath will be
      # kept and the file will have a new name
      newParentPath = parentAbsolutePath
      newPathName = newPath

    currentAbsolutePath = this._makeAbsolutePath parentAbsolutePath, currentPathName
    newAbsolutePath = this._makeAbsolutePath newParentPath, newPathName

    path.incrementProperty 'loadCount'
    sftpSocket.rename currentAbsolutePath, newAbsolutePath, (err) =>
      if err
        if err == 'aio:name_taken'
          App.set 'flashMessage', { type: 'error', text: "The name “#{newPathName}” is already taken. Please choose a different name." }
        else
          App.set 'flashMessage', { type: 'error', text: 'Rename failed.' }
      else
        path.set 'absolutePath', newAbsolutePath

      path.decrementProperty 'loadCount'
      path.set 'editable', false

      this.refresh parentPath
      this.refresh newPath if isDrag

  resetRenamedPath: (path) ->
    path.set 'editable', false

  showUploadDialog: (path) ->
    path ?= @get 'rootPath'
    App.util.mixpanel 'Opened File Upload Dialog'
    @set 'uploadDialog', App.UploadDialog.create path: path

  beginUpload: ->
    uploadDialog = @get 'uploadDialog'
    if uploadDialog && uploadDialog.get('length')
      App.authToken.obtain 'rainbow', (err, authToken) ->
        return if err
        uploadDialog.set 'uploading', true
        uploadDialog.set 'authToken', authToken
        uploadDialog.forEach (upload) ->
          upload.submit()
    else
      @set 'uploadDialog', null

  sendUploadToken: (path, token) ->
    sftpSocket = @get 'sftpSocket'
    sftpSocket.sendUploadToken path, token, _.bind(@didSendUploadToken, this)

  didSendUploadToken: (err, absolutePath) ->
    path = App.Path.create absolutePath: absolutePath
    this.refreshPath path.get('parentAbsolutePath')
    this.didFinishAnUpload()

  didFinishAnUpload: ->
    uploadDialog = @get 'uploadDialog'
    if uploadDialog && uploadDialog.get 'completed'
      App.util.mixpanel 'Uploaded File'
      @set 'uploadDialog', null

  confirmDelete: (path) ->
    @set 'confirmDeletePathDialog', App.ConfirmDeletePathDialog.create path: path

  delete: (path) ->
    return if path == @get 'rootPath'
    deleteMethod = if path.get('directory') then 'deleteDir' else 'deleteFile'
    sftpSocket = @get 'sftpSocket'
    homePath = @get 'homePath'
    parentPath = homePath.findByRelativePath path.get('parentAbsolutePath')
    path.set 'beingDeleted', true
    path.incrementProperty 'loadCount'

    sftpSocket[deleteMethod] path.get('absolutePath'), (err) =>
      if err
        path.decrementProperty 'loadCount'
        path.set 'beingDeleted', false
      else
        path.set 'deleted', true
        if parentPath
          parentPath.get('content').removeObject path
        # TODO: not strictly necessary but, remove all children paths recursively from 'content' too?
        this.get('content').removeObject path

  moveDraggedPathTo: (path) ->
    draggedPath = @get 'currentlyDraggedPath'
    absoluteDroppablePath = path.get 'absolutePath'
    #expand the droppable path
    path.set 'expanded', true
    this.applyRenamedPath draggedPath, path, true
    @set 'currentlyDraggedPath', null
    @set 'currentDroppable', null

  _makeAbsolutePath: (parentAbsolutePath, fileName) ->
    _.chain([parentAbsolutePath, fileName]).compact().join('/').value()

App.IdeSidebarController.reopenClass
  REFRESH_DELAY: 200

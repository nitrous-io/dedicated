App.IdeFileFinderController = Em.Controller.extend
  needs: ['ide', 'ideEditor', 'ideSidebar']

  opsSocketBinding: Em.Binding.oneWay 'controllers.ide.opsSocket'
  fileFinderDialog: null

  initialize: ->
    socket = @get 'opsSocket'
    socket.on 'file_find.output', _.bind(@onOutput, this)
    socket.on 'file_find.error', _.bind(@onError, this)
    socket.on 'file_find.completed', _.bind(@onCompleted, this)

  reset: ->
    @set 'fileFinderDialog', null
    socket = @get 'opsSocket'
    socket.off 'file_find.output'
    socket.off 'file_find.error'

  onOutput: (error, data) ->
    @get('fileFinderDialog')?.addResults(data.term, data.root, data.paths)

  onError: (error, data) ->
    dialog = @get 'fileFinderDialog'
    return unless dialog?

    # handle errors occurr before search begins
    if error
      dialog.set 'error', error

    # when a mid-stream error occurrs,
    # show current results and mark the search as complete
    if data.error
      dialog.searchComplete(data.term, data.root)

  onCompleted: (error, data) ->
    @get('fileFinderDialog')?.searchComplete(data.term, data.root)

  showFindDialog: ->
    path = this.get('controllers.ideSidebar.rootPath')?.absolutePath
    path = App.get('container.homePath') if !path?
    @set 'fileFinderDialog', App.FileFinderDialog.create basePath: path

  startFind: (pattern, path) ->
    dialog = @get 'fileFinderDialog'
    socket = @get 'opsSocket'
    return if !dialog || !socket

    path = App.get('container.homePath') if not path?.length

    dialog.find pattern, path, =>
      @set 'findPattern', pattern
      @set 'findPath', path
      Ember.run.debounce this, this.socketFind, 500 # at most once per 0.5s

  socketFind: ->
    dialog = @get 'fileFinderDialog'
    socket = @get 'opsSocket'
    pattern = @get 'findPattern'
    path = @get 'findPath'

    if dialog && socket && pattern && path
      dialog.searchStart(pattern, path)
      socket.fileFind pattern, path, (err) ->
        if err
          dialog.set 'error', err
          return

  closeFindDialog: ->
    this.reset()

  getFindPathFrom: (basePath, level = 2) ->
    return nil unless editorController = @get 'controllers.ideEditor'

    # No current active tab
    path = editorController.get('activeTab.path')
    return nil unless path

    currentFilePath = path.absolutePath
    # When README.md is open
    return nil unless currentFilePath

    startIndex = currentFilePath.indexOf(basePath)

    # When the file is not in findPath
    return nil if startIndex == -1

    relativePath = currentFilePath.substring(basePath.length + startIndex)
    folders = relativePath.split('/')
    # Remove first empty string, current folder and filename
    folders = folders.slice(1, (folders.length - 2))

    searchFolder = basePath

    paths = []
    for folder, i in folders
      searchFolder = "#{searchFolder}/#{folder}"
      paths.unshift searchFolder

    results = []
    for path, i in paths
      results.push path if i % level == 0

    return results[0]

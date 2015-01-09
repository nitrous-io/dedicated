App.IdeFileFinderController = Em.Controller.extend
  needs: ['index', 'ideEditor', 'ideSidebar']

  opsSocketBinding: Em.Binding.oneWay 'controllers.index.opsSocket'
  fileFinderDialog: null
  workspaceDir: 'workspace'

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
    @get('fileFinderDialog').addResults(data.term, data.root, data.paths)

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
    @get('fileFinderDialog').searchComplete(data.term, data.root)

  showFindDialog: (path) ->
    if !path?
      path = this.guessPath(@get('controllers.ideEditor.activeTab.path.absolutePath'))
    @set 'fileFinderDialog', App.FileFinderDialog.create basePath: path
    App.util.mixpanel 'Started FileFinder',
      basePath: path

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
      socket.fileFind pattern, path, -> {}

  closeFindDialog: ->
    this.reset()

  # try to pick a sensible default search root
  guessPath: (path) ->
    homedir = App.get('container.homePath')
    return homedir if not path?.length

    path = path.replace(new RegExp("^#{homedir}\/"), '')
    parts = path.split('/')
    in_workspace = parts[0] == @get('workspaceDir')
    path = switch
      when parts.length == 1
        if in_workspace then @get('workspaceDir') else homedir
      when parts.length == 2
        parts[0]
      else
        if in_workspace
          [parts[0], parts[1]].join('/')
        else
          parts[0]
    [homedir, path].join("/")

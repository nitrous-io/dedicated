App.IdeFileFinderController = Em.Controller.extend
  needs: ['boxIde', 'ideEditor', 'ideSidebar']

  fileFinderDialog: null

  reset: ->
    @set 'fileFinderDialog', null

  showFindDialog: (path) ->
    if !@get('controllers.boxIde.commandSocket.started')
      App.util.connectionError()
      return

    if !path?
      path = this.guessPath(@get('controllers.ideEditor.activeTab.path.absolutePath'))
    @set 'fileFinderDialog', App.FileFinderDialog.create basePath: path
    App.util.mixpanel 'Started FileFinder',
      basePath: path

  startFind: (pattern, path) ->
    dialog = @get 'fileFinderDialog'
    socket = @get 'controllers.boxIde.commandSocket'
    return if !dialog || !socket

    path = '.' if not path?.length

    dialog.find pattern, path, =>
      @set 'findPattern', pattern
      @set 'findPath', path
      Ember.run.debounce this, this.socketFind, 500 # at most once per 0.5s

  socketFind: ->
    dialog = @get 'fileFinderDialog'
    socket = @get 'controllers.boxIde.commandSocket'
    pattern = @get 'findPattern'
    path = @get 'findPath'
    if dialog && socket && pattern && path
      dialog.searchStart(pattern, path)
      socket.filefind dialog, pattern, path, -> {}

  closeFindDialog: ->
    this.reset()

  # try to pick a sensible default search root
  guessPath: (path) ->
    homedir = '.'

    return homedir if not path?.length
    parts = path.split('/')
    in_workspace = parts[0] == 'workspace'
    switch
      when parts.length == 1
        if in_workspace then 'workspace' else homedir
      when parts.length == 2
        parts[0]
      else
        if in_workspace
          [parts[0], parts[1]].join('/')
        else
          parts[0]

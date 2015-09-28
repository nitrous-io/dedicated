class @PluginBase
  constructor: (@document, options = {}) ->

  setOption: (key, value)->
    @[key] = value

    if key == 'opsSocket'
      @resolvePath()
      value.on 'fileChanged.output', _.bind(@handleFileChanged, this)
      value.on 'dirChanged.output', _.bind(@handleDirChanged, this)

    if key == 'codeMirror'
      @afterSetCodeMirror()

  afterInitialize: ->
    @resolvePath()

  afterSave: (context)->
  afterSetCodeMirror: (content)->
  afterChanges: (changes)->

  handleFileChanged: (change, file)->
  handleDirChanged: (change, dir)->

  supportsGoToDefinition: false
  goToDefinition: (context)->

  path: ->
    @document.get 'path'

  getRootPath: ->
    App.get('router.ideSidebarController.rootPath')

  eachLine: (callback)->
    if @codeMirror
      @codeMirror.eachLine (line)=>
        callback(line)

  cTagsLocation: ->

  searchCTags: (searchTerm, callback)->
    cmd = [
      "/usr/bin/ag",
      "--nocolor",
      "--no-numbers",
      searchTerm
      @cTagsLocation()
    ]
    cwd = ''
    env = ['']
    parentRelDirectory = @relativePath.split('/').slice(0, -1).join('/')
    if @autoCompleteRun
      window.clearTimeout(@autoCompleteRun)
      @autoCompleteRun = null

    @autoCompleteRun = window.setTimeout =>
      @autoCompleteRun = null
      @exec cmd, cwd, env, (err, cmdErr, stdout, stderr) =>
        unless stdout
          return

        callback(err, cmdErr, stdout, stderr)
      , minimal: true
    , 50

  parseCTags: (data, callback)->
    lines = data.split(/\r?\n/)
    for line in lines
      parts = line.split(/\t/)
      if parts && parts.length >= 2
        varName = parts[0]
        if varName
          path = parts[1]
          callback(varName, path, parts)

  resolvePath: ->
    path = @document.get('path.absolutePath')
    return unless path
    cmd = [
      "readlink -f #{path}",
    ]
    cwd = ''
    env = []
    @exec cmd, cwd, env, (err, cmdErr, stdout, stderr) =>
      if stdout
        absPath = stdout.trim()
        @absolutePath = App.Path.create(absolutePath: absPath)
        @relativePath = absPath.replace(@getRootPath().get('absolutePath'), "").replace(/^\//,"")
    , nologin: true

  exec: (cmd, cwd, env, callback, options = {})->
    return unless @opsSocket
    execCmd = @opsSocket.run
    if options.minimal == true && App.get('freighterdFeatures')?.contains('minirun')
      execCmd = @opsSocket.minirun
    else if options.nologin != true && App.get('freighterdFeatures')?.contains('loginrun')
      execCmd = @opsSocket.loginrun

    execCmd.apply @opsSocket, [cmd, cwd, env, callback]

  handleSnackbarError: (err)->
    App.postError err

  handleError: (err, opts = {})->
    PubSub.publish 'error.message',
      App.ErrorDialog.create
        errorMessage: err
        redirectUrl: opts.url
        newWindow: true
        errorTitle: (opts.title || @document.get('title'))
        okButtonText: (opts.okText || 'OK')

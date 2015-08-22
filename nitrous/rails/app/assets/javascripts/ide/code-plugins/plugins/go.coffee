#= require ./base

GOPATH_ERROR = /not beneath any of these GOROOT\/GOPATH directories:(.+)$/

klass = class Go extends PluginBase
  afterSave: (context)->
    opsSocket = context.opsSocket
    sidebar = context.sidebar
    pth = @path()

    if !pth || !opsSocket
      return
    cmd = [
      "goimports",
      "-w",
      pth.get('absolutePath')
    ]
    cwd = ''
    env = []
    opsSocket.run cmd, cwd, env, (err, cmdErr, stdout, stderr) =>
      @document.processErrMessage stderr
      if err || cmdErr
        @document.set 'clean', true
        return
      if sidebar
        sidebar.refreshTextFileContent pth
  
  supportsGoToDefinition: true
  goToDefinition: (context)->
    opsSocket = context.opsSocket
    sidebar = context.sidebar
    pth = @path()

    if !pth || !opsSocket
      return
    cmd = [
      "oracle",
      "-format=json",
      "-pos=$(readlink -f #{pth.get('absolutePath')}):##{@document.get("currentOffset")}",
      "definition"
    ]
    cwd = ''
    env = []
    opsSocket.run cmd, cwd, env, (err, cmdErr, stdout, stderr) =>
      if err || cmdErr
        if stderr
          @processOracleErr(stderr)
        return
      data = JSON.parse stdout
      if (def = data.definition) && (pos = def.objpos)
        match = pos.match(/^([^:]+):(\d+):(\d+)$/)
        if match
          filepath = match[1]
          line = match[2]
          cursor = match[3]
          pth = App.Path.create absolutePath: filepath
          if sidebar
            pos = {line: parseInt(line)-1, ch: parseInt(cursor)-1}
            sidebar.openPath pth, selections: [{anchor: pos, head: pos}]

  processOracleErr: (err)->
    err = err.trim()
    if m = err.match GOPATH_ERROR
      @handleError("The file is outside of your GOPATH/GOROOT: #{m[1]}")

App.CodePluginsManager.registerCodePlugin "golang", klass, ["go", "text/x-go"]
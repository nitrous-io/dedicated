class @GlobalPluginBase
  SOFTWARE_REQUIREMENTS = "1"
  SOFTWARE_REQUIRED = "silversearcher-ag exuberant-ctags"

  TYPE = "base"

  constructor: () ->
    @activeTimers = {}

  setOption: (key, value)->
    @[key] = value

  afterInitialize: ->
    return if @container?.get('metadata.softwareRequirements') == SOFTWARE_REQUIREMENTS

    return if App.CodePluginsManager.installingSoftware
    App.CodePluginsManager.installingSoftware = true
    cmd = [
      "sudo apt-get install -y #{SOFTWARE_REQUIRED}"
    ]
    @opsSocket.loginrun cmd, '', [], (err, cmdErr, stdout, stderr) =>
      if stdout.match(/is already the newest version/) || stdout.match(/Setting up/)
        @container?.setMetadata(softwareRequirements: SOFTWARE_REQUIREMENTS)

        for plugin in (App.CodePluginsManager.activePlugins[TYPE] || [])
          plugin.triggerCtagsUpdate()

        console.log "#{SOFTWARE_REQUIRED} was installed", stdout
      else
        App.postError('An error occurred installing ag/ctags.')
        console.log "#{SOFTWARE_REQUIRED} was not installed", stderr, stdout

      App.CodePluginsManager.installingSoftware = false

  unload: ->
    if @activeTimers
      for path, timer of @activeTimers
        window.clearTimeout timer

  triggerCtagsUpdate: (parent, callback)->
    return unless @opsSocket

    return if @activeTimers[parent]

    @activeTimers[parent] = window.setTimeout =>
      @runCtags(parent)
    , 50

  runCtags: (parent)->

  getHomePath: ->
    return App.get('container.homePath')
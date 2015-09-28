#= require ./base

klass = class GlobalRuby
  NITROUS_IMAGE_CMD = "ctags -f .ruby-tags -R --languages=ruby --exclude=.git --exclude=log --exclude=tmp . $(LD_LIBRARY_PATH=$(dirname $(rbenv which ruby))/../lib bundle list --paths)"
  CMD = "ctags -f .ruby-tags -R --languages=ruby --exclude=.git --exclude=log --exclude=tmp . $(bundle list --paths)"

  constructor: () ->
    @activeTimers = {}

  setOption: (key, value)->
    @[key] = value

  triggerCtagsUpdate: (parent)->
    return unless @opsSocket

    return if @activeTimers[parent]
    cmd = CMD
    if @container.image.match(/nitrous/) || @userSettings?.nitrousRuby
      cmd = NITROUS_IMAGE_CMD

    @activeTimers[parent] = window.setTimeout =>
      cmd = [
        "cd #{parent} && #{CMD}"
      ]
      @opsSocket.loginrun cmd, parent, [], (err, cmdErr, stdout, stderr) =>
        @activeTimers[parent] = null
    , 50

App.CodePluginsManager.registerGlobalPlugin "ruby", klass
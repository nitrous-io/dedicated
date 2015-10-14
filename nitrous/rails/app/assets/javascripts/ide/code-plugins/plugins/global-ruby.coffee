#= require ./global-plugin-base

klass = class GlobalRuby extends GlobalPluginBase
  TYPE = 'ruby'

  NITROUS_IMAGE_CMD = "ctags -f .ruby-tags -R --languages=ruby --exclude=.git --exclude=log --exclude=tmp . $(LD_LIBRARY_PATH=$(dirname $(rbenv which ruby))/../lib bundle list --paths)"
  CMD = "ctags -f .ruby-tags -R --languages=ruby --exclude=.git --exclude=log --exclude=tmp . $(bundle list --paths)"

  afterInitialize: ->
    @ctagsCmd = CMD
    if @container?.image.match(/nitrous/) || @container?.get('metadata.nitrousRuby')
      ctagsCmd = NITROUS_IMAGE_CMD
    super

  runCtags: (parent)->
    @opsSocket.loginrun [@ctagsCmd], parent, [], (err, cmdErr, stdout, stderr) =>
      @activeTimers[parent] = null

      if cmdErr || err
        console.log "Error invoking ctags update:", err, cmdErr, stderr, stdout
        return

      for plugin in (App.CodePluginsManager.activePlugins['ruby'] || [])
        plugin.ctagsUpdated()

App.CodePluginsManager.registerGlobalPlugin "ruby", klass
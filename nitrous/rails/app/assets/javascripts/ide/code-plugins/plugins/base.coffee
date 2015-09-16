class @PluginBase
  constructor: (@document) ->

  afterSave: (context)->

  supportsGoToDefinition: false
  goToDefinition: (context)->

  path: ->
    @document.get 'path'

  exec: (context, cmd, cwd, env, callback)->
    execCmd = context.opsSocket.run
    if App.get('freighterdFeatures')?.contains('loginrun')
      execCmd = context.opsSocket.loginrun

    execCmd.apply context.opsSocket, [cmd, cwd, env, callback]

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

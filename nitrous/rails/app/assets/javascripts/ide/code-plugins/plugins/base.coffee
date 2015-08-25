class @PluginBase
  constructor: (@document) ->

  afterSave: (context)->

  supportsGoToDefinition: false
  goToDefinition: (context)->

  path: ->
    @document.get 'path'

  handleSnackbarError: (err)->
    App.Snackbar.postError err

  handleError: (err, opts = {})->
    PubSub.publish 'error.message',
      App.ErrorDialog.create
        errorMessage: err
        redirectUrl: opts.url
        newWindow: true
        errorTitle: (opts.title || @document.get('title'))
        okButtonText: (opts.okText || 'OK')
containerStoppedMessage = 'Container is not running. Please start your container first.'

App.TerminalController = Em.ObjectController.extend
  eventsSocket: null
  errorDialog: null

  initialize: ->
    if container = @get 'content'
      router = App.get 'router'
      router.get('ideLayoutController').initialize()
      router.get('ideMainMenuController').initialize()
      router.get('ideConsoleController').initialize()

      App.set 'eventsSocket', evSocket = App.EventsSocket.create()
      evSocket.connect()

      evSocket.on 'container.stopped', (data)=>
        if @get('content.id') == data?.container?.id
          @handleContainerStopped()

      evSocket.on 'container.reconfigured', (data)=>
        if @get('content.id') == data?.container?.id
          @handleContainerReconfigured()

  reset: ->
    router = App.get 'router'
    router.get('ideConsoleController').reset()
    router.get('ideMainMenuController').reset()
    router.get('ideLayoutController').reset()

    @set 'content', null
    @set 'errorDialog', null

  # TODO: this code is already used in IDE, refactor
  handleContainerReconfigured: ->
    App.snackbar.post "Container reconfigured, reconnecting...."
    App.reloadContainer().then (v)=>
      ops = @get 'opsSocket'
      ops.disconnect()
      ops.set 'container', App.get 'container'
      ops.connect()
    , (err)->
      App.snackbar.post "Error reconnecting to container after reconfig!"

  handleContainerStopped: ->
    slug = App.get 'slug'
    dialog = App.ErrorDialog.create
      errorMessage: containerStoppedMessage
      redirectUrl: "/app#/containers/#{slug}"
      okButtonText: 'Back to Dashboard'

    @set('errorDialog', dialog)

  validateContainerState: ->
    container = @get 'content'

    if container.state != 'running'
      @handleContainerStopped()

  warnWhenOffline: (->
    if App.get 'offline'
      App.set 'flashMessage', { type: 'alert-error', text: "Unable to connect to the server. Changes will not be saved.", sticky: true }
    else
      App.set 'flashMessage', null
  ).observes 'App.offline'

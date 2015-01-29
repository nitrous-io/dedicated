containerStoppedMessage = 'Container is not running. Please start your container first.'

App.IdeController = Em.ObjectController.extend
  needs: ['ideFindInProject', 'ideFileFinder']

  eventsSocket: null
  opsSocket: null
  opsSocketReady: null
  opsTimeoutAlert: null
  errorDialog: null

  opsConnectionGracePeriod: 15 * 1000 # TODO: This should come from a KeyValue object from the server-side
  opsConnectionLost: false

  opsSocketReadyBinding: Em.Binding.oneWay 'opsSocket.started'

  opsSocketNotReady: Em.computed.not 'opsSocketReady'

  workspaceDir: 'code'

  initialize: ->
    if container = @get 'content'
      router = App.get 'router'
      router.get('ideLayoutController').initialize()
      router.get('ideMainMenuController').initialize()
      router.get('ideSidebarController').initialize()
      router.get('ideEditorController').initialize()
      router.get('ideConsoleController').initialize()

      @set 'opsSocket', opsSocket = App.OpsSocket.create(container: container)
      opsSocket.connect()

      App.set 'eventsSocket', evSocket = App.EventsSocket.create()
      evSocket.connect()

      evSocket.on 'container.stopped', (data)=>
        if @get('content.id') == data?.container?.id
          @handleContainerStopped()

      evSocket.on 'container.reconfigured', (data)=>
        if @get('content.id') == data?.container?.id
          @handleContainerReconfigured()

  reset: ->
    if opsSocket = @get 'opsSocket'
      opsSocket.disconnect()
      opsSocket.destroy()

    router = App.get 'router'
    router.get('ideConsoleController').reset()
    router.get('ideEditorController').reset()
    router.get('ideSidebarController').reset()
    router.get('ideMainMenuController').reset()
    router.get('ideLayoutController').reset()

    @set 'opsSocket', null
    @set 'content', null
    @set 'errorDialog', null

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

  opsSocketDisconnected: (->
    unless @get('opsSocketReady')
      opsTimeoutAlert = Em.run.later this, (->
        @set 'opsConnectionLost', true
      ), @get('opsConnectionGracePeriod')

      @set 'opsTimeoutAlert', opsTimeoutAlert
  ).observes 'opsSocketReady'

  opsSocketConnected: (->
    if @get('opsSocketReady') && (opsTimeoutAlert = @get('opsTimeoutAlert'))
      Em.run.cancel opsTimeoutAlert
      @set 'opsConnectionLost', false
  ).observes 'opsSocketReady'

  warnWhenOffline: (->
    if App.get 'offline'
      App.set 'flashMessage', { type: 'alert-error', text: "Unable to connect to the server. Changes will not be saved.", sticky: true }
    else
      App.set 'flashMessage', null
  ).observes 'App.offline'

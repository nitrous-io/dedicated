App.IdeController = Em.ObjectController.extend
  needs: ['ideFindInProject', 'ideFileFinder']

  eventsSocket: null
  opsSocket: null
  commandSocket: null
  opsSocketReady: null
  opsTimeoutAlert: null
  repository: null

  opsConnectionGracePeriod: 15 * 1000 # TODO: This should come from a KeyValue object from the server-side
  opsConnectionLost: false

  opsSocketReadyBinding: Em.Binding.oneWay 'opsSocket.started'
  commandSocketReadyBinding: Em.Binding.oneWay 'commandSocket.started'

  opsSocketNotReady: Em.computed.not 'opsSocketReady'
  commandSocketNotReady: Em.computed.not 'commandSocketReady'

  workspaceDir: 'code'

  initialize: ->
    if content = @get 'content'
      #@set 'commandSocket', commandSocket = App.CommandSocket.create box: content
      router = App.get 'router'
      router.get('ideLayoutController').initialize()
      router.get('ideMainMenuController').initialize()
      router.get('ideSidebarController').initialize()
      router.get('ideEditorController').initialize()
      router.get('ideConsoleController').initialize()

      @set 'opsSocket', opsSocket = App.OpsSocket.create(container: content)
      opsSocket.connect()

      App.set 'eventsSocket', evSocket = App.EventsSocket.create()
      evSocket.connect()
      evSocket.on 'container.reconfigured', (data)=>
        if App.get('container.id') == data?.container?.id
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
    @set 'repository', null
    @set 'content', null

  handleContainerReconfigured: ->
    App.snackbar.post "Container reconfigured, reconnecting...."
    App.reloadContainer().then (v)=>
      ops = @get 'opsSocket'
      ops.disconnect()
      ops.set 'container', App.get 'container'
      ops.connect()
    , (err)->
      App.snackbar.post "Error reconnecting to container after reconfig!"

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

      if repo = @get 'repository'
        if repo.get('state') == 'failed'
          cacheKey = "repo:#{repo.get('id')}:shownError"
          if !App.localStorage.get(cacheKey)
            App.set 'flashMessage', { type: 'error', text: "Cloning the repository failed: #{repo.get('failure_reason')}" }
            App.localStorage.set(cacheKey, true)
  ).observes 'opsSocketReady'

  warnWhenOffline: (->
    if App.get 'offline'
      App.set 'flashMessage', { type: 'alert-error', text: "Unable to connect to the server. Changes will not be saved.", sticky: true }
    else
      App.set 'flashMessage', null
  ).observes 'App.offline'

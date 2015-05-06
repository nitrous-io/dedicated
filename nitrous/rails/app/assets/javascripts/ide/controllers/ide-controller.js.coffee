containerStoppedMessage = 'Container is not running. Please start your container first.'

App.IdeController = Em.ObjectController.extend
  needs: ['ideFindInProject', 'ideFileFinder']

  opsSocket: null
  collabSocket: null
  collabEnabled: true

  opsTimeoutAlert: null
  repository: null
  errorDialog: null
  freighterFeatures: null

  opsConnectionGracePeriod: 15 * 1000 # TODO: This should come from a KeyValue object from the server-side
  opsConnectionLost: false

  opsSocketReadyBinding: Em.Binding.oneWay 'opsSocket.started'
  opsSocketNotReady: Em.computed.not 'opsSocketReady'

  collabSocketReadyBinding: Em.Binding.oneWay 'collabSocket.started'
  collabSocketNotReady: Em.computed.not 'collabSocketReady'

  ideSocketsReady: (->
    @get('opsSocketReady') && ((@get('collabEnabled') && @get('collabSocketReady')) || !@get('collabEnabled'))
  ).property 'opsSocketReady', 'collabSocketReady', 'collabEnabled'

  ideSocketsNotReady: Em.computed.not 'ideSocketsReady'

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

      App.CollabSocket.checkAvailability container, (ok) =>
        @set 'collabEnabled', ok
        if ok
          @set 'collabSocket', collabSocket = App.CollabSocket.create(container: container)
          collabSocket.connect()

      App.set 'eventsSocket', evSocket = App.EventsSocket.create()
      evSocket.connect()

      ['container.stopped', 'container.restarted', 'container.reconfigured'].forEach (evt) =>
        evSocket.on evt, (data) =>
          if @get('content.id') == data?.container?.id
            App.get('container').setProperties(data.container)
            if data.config
              App.get('container').set('config', data.config)

      evSocket.on 'container.stopped', (data) =>
        if @get('content.id') == data?.container?.id
          @handleContainerStopped()

      evSocket.on 'container.restarted', (data) =>
        if @get('content.id') == data?.container?.id
          App.snackbar.post "Container restarted, reconnecting..."

      evSocket.on 'container.reconfigured', (data) =>
        if @get('content.id') == data?.container?.id
          App.snackbar.post "Container reconfigured, reconnecting..."

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
    @set 'errorDialog', null

  freighterSupports: (feature) ->
    if features = @get 'freighterFeatures'
      return features[feature]
    false

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

      if repo = @get 'repository'
        if repo.get('state') == 'failed'
          cacheKey = "repo:#{repo.get('id')}:shownError"
          if !App.localStorage.get(cacheKey)
            App.set 'flashMessage', { type: 'error', text: "Cloning the repository failed: #{repo.get('failure_reason')}" }
            App.localStorage.set(cacheKey, true)

      socket = @get('opsSocket')
      socket.getFeatures (err, features) =>
        if !err
          @set 'freighterFeatures', features
  ).observes 'opsSocketReady'

  warnWhenOffline: (->
    if App.get 'offline'
      App.set 'flashMessage', { type: 'alert-error', text: "Unable to connect to the server. Changes will not be saved.", sticky: true }
    else
      App.set 'flashMessage', null
  ).observes 'App.offline'

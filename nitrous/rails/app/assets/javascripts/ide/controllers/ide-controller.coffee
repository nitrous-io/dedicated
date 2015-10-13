containerStoppedMessage = 'Container is not running. Please start your container first.'
hostStoppedMessage = 'Workspace is not running. Please start your workspace first, then start your container.'
resumingSessionMessage = 'Resuming Session...'
hostRestartingMessage = 'Workspace resuming, please wait...'
containerRestartingMessage = 'Container resuming, please wait...'
freighterFeaturesRetry = 3000 # 3s

App.IdeController = Em.ObjectController.extend
  needs: ['ideFindInProject', 'ideFileFinder']

  opsSocket: null
  collabSocket: null
  collabEnabled: true

  hostStarted: false
  hostRestarted: false
  containerStarted: false
  containerRestarted: false

  resumingSession: false

  opsTimeoutAlert: null
  repository: null
  errorDialog: null
  showBetaDialog: null
  resumeSessionDialog: null

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
      # TODO fix once chrome app supports www
      if window.location.hostname.match(/^pro\./) && !App.get('chromeApp')
        App.postError "We are changing our default domain from pro.nitrous.io to www.nitrous.io. Please login and switch to www.nitrous.io"

      router = App.get 'router'
      router.get('ideLayoutController').initialize()
      router.get('ideMainMenuController').initialize()
      router.get('ideSidebarController').initialize()
      router.get('ideEditorController').initialize()
      router.get('ideConsoleController').initialize()

      PubSub.subscribe 'error.message', (msg, data) =>
        @set('errorDialog', data)

      App.CodePluginsManager.setAvailableOption 'container', container

      @set 'opsSocket', opsSocket = App.OpsSocket.create(container: container)
      opsSocket.connect()

      App.set 'eventsSocket', evSocket = App.EventsSocket.create()
      evSocket.connect()

      pingerSocket = App.PingerSocket.create(hostId: container.get('host_id'), alert: container.get('host')?.auto_shutdown)
      App.set 'pingerSocket', pingerSocket
      pingerSocket.connect()

      @loadFreighterFeatures(container)

      ['container.stopped', 'container.restarted', 'container.reconfigured'].forEach (evt) =>
        evSocket.on evt, (data) =>
          if @get('content.id') == data?.container?.id
            App.get('container').setProperties(data.container)
            if data.config
              App.get('container').set('config', data.config)

      evSocket.on 'host.started', (data) =>
        @set('hostStarted', true)

      evSocket.on 'host.restarted', (data) =>
        @set('resumingSession', true)
        @set('hostRestarted', true)
        @set('resumeSessionDialog', @resumeSessionErrDialog(resumingSessionMessage))

        @startContainer(App.container.slug).then =>
          @set('containerStarted', true)
        , (jqxhr) =>
          @set('errorDialog', @containerStoppedErrDialog())

      evSocket.on 'container.started', (data) =>
        if @get('content.id') == data?.container?.id
          @set('containerStarted', true)
          App.snackbar.post "Container started, resuming session..."

      evSocket.on 'container.restarted', (data) =>
        if @get('content.id') == data?.container?.id
          @set('containerRestarted', true)
          App.snackbar.post "Container restarted, resuming session..."

      evSocket.on 'container.stopped', (data) =>
        # Don't interrupt auto start container for user on host restart scenario.
        unless @resumingSession
          if @get('content.id') == data?.container?.id
            @set('errorDialog', @containerStoppedErrDialog())

      evSocket.on 'container.reconfigured', (data) =>
        if @get('content.id') == data?.container?.id
          App.snackbar.post "Container reconfigured, reconnecting..."

      ['collaborator.added', 'collaborator.removed'].forEach (evt) =>
        evSocket.on evt, (data) =>
          if @get('content.id') == data?.container?.id
            App.get('container').loadCollaborators()

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
    @set 'resumeSessionDialog', null

  validateHostAndContainerState: ->
    if App.container.host.state != 'running'
      switch App.container.host.state
        when 'stopped'
          @handleHostStopped()
        when 'restarting'
          @set('resumeSessionDialog', @resumeSessionErrDialog(hostRestartingMessage))
        else
          @set('errorDialog', @hostStoppedErrDialog())
    else if App.container.state != 'running'
      switch App.container.state
        when 'stopped'
          @handleContainerStopped()
        when 'restarting'
          @set('resumeSessionDialog', @resumeSessionErrDialog(containerRestartingMessage))
        else
          @set('errorDialog', @containerStoppedErrDialog())

  handleHostStopped: ->
    @set('resumeSessionDialog', @resumeSessionErrDialog(resumingSessionMessage))

    @startHost(App.container.host_id).then =>
      ###
      Even though host and container state values are set to true on
      receiving the relevant event, we additionally set these values
      on successful POST requests so that we don't miss the state
      changes if for some reason the event is not received.
      ###
      @set('hostStarted', true)
      @startContainer(App.container.slug).then =>
        @set('containerStarted', true)
      , (jqxhr) =>
        @set('errorDialog', @containerStoppedErrDialog())
    , (jqxhr) =>
      @set('errorDialog', @hostStoppedErrDialog())

  handleContainerStopped: ->
    @set('resumeSessionDialog', @resumeSessionErrDialog(resumingSessionMessage))
    @startContainer(App.container.slug).then =>
      @set('containerStarted', true)
    , (jqxhr) =>
      @set('errorDialog', @containerStoppedErrDialog())

  startHost: (host_id)->
    Em.RSVP.Promise.cast Em.$.ajax
      type: 'POST',
      url: "/api/v0/hosts/#{host_id}/start",
      headers: { Authorization: "Bearer #{App.get 'token'}" }

  startContainer: (slug)->
    Em.RSVP.Promise.cast Em.$.ajax
      type: 'POST',
      url: "/api/v0/containers/#{slug}/start",
      headers: { Authorization: "Bearer #{App.get 'token'}" }

  resumeSessionErrDialog: (msg)->
    App.ResumeSessionDialog.create
      message: msg

  hostStoppedErrDialog: ->
    App.ErrorDialog.create
      errorMessage: hostStoppedMessage
      redirectUrl: "/app/#/workspaces/#{App.container.host.slug}"
      okButtonText: 'Back to Dashboard'

  containerStoppedErrDialog: ->
    App.ErrorDialog.create
      errorMessage: containerStoppedMessage
      redirectUrl: "/app/#/containers/#{App.container.slug}"
      okButtonText: 'Back to Dashboard'

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

  collabSocketDidStartForTheFirstTime: (->
    if @get 'collabSocket.startedFirstTime'
      fileMon = App.ActiveFilesMonitor.create(collabSocket: @get('collabSocket'), username: App.get('user.username'))
      @set 'activeFilesMonitor', fileMon
  ).observes 'collabSocket.startedFirstTime'

  opsSocketDidStartForTheFirstTime: (->
    if @get 'opsSocket.startedFirstTime'
      socket = @get 'opsSocket'
      App.CodePluginsManager.setAvailableOption 'opsSocket', socket
  ).observes 'opsSocket.startedFirstTime'

  warnWhenOffline: (->
    if App.get 'offline'
      App.set 'flashMessage', { type: 'alert-error', text: "Unable to connect to the server. Changes will not be saved.", sticky: true }
    else
      App.set 'flashMessage', null
  ).observes 'App.offline'

  loadFreighterFeatures: (container)->
    @getFreighterFeatures(container).then =>
      if @loadFreighterFeaturesRetry
        window.clearTimeout @loadFreighterFeaturesRetry
      features = App.get('freighterdFeatures') ? []

      if features.contains('collab')
        @set 'collabEnabled', true
        @set 'collabSocket', collabSocket = App.CollabSocket.create(container: container)
        collabSocket.connect()

      if features.contains('ping')
        App.set 'pinger', pinger = App.Pinger.create(container: container)
    , (jqxhr) =>
      return if @loadFreighterFeaturesRetry
      @loadFreighterFeaturesRetry = window.setTimeout(=>
        @loadFreighterFeaturesRetry = null
        @loadFreighterFeatures(container)
      , freighterFeaturesRetry)

  getFreighterFeatures: (container) ->
    protocol = location.protocol
    freighterdUrl = container.get 'freighterdUrl'
    url = "#{protocol}//#{freighterdUrl}/features"

    Em.RSVP.Promise.cast(Em.$.ajax(
      type: 'GET'
      url: url
      dataType: 'json'
    )).then (data) ->
      if Em.isArray(data.features)
        App.set 'freighterdFeatures', data.features

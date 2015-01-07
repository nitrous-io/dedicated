transformEnvHashToArray = (hash) ->
  envVars = []
  if hash
    for own key, value of hash
      envVars.push("#{key}=#{value}") if value
  envVars

portMap = (port) ->
    ['public', port, port].join(':')

App.AppPortsController = Em.ObjectController.extend
  publicPorts: Em.computed.alias 'model.appContainer.publicPorts'
  saving: false
  port: null
  previewPort: false
  removePreviewPort: false
  closeApplyDialog: false

  clear: ->
    @_super.apply this, arguments
    @setProperties
      saving: false
      port: null
      previewPort: false
      removePreviewPort: false
      closeApplyDialog: false

  actions:
    add: ->
      port = @get 'port'
      container = @get('model.appContainer')
      portBindings = container.get('config.port')
      if @get('previewPort')
        container.set 'newPreviewPort', port
      pb = portMap port
      unless portBindings.contains(pb)
        portBindings.pushObject(pb)
      @set 'port', null
      @set 'previewPort', false

    remove: (port) ->
      pb = portMap port.value
      container = @get('model.appContainer')
      container.get('config.port').removeObject pb
      if port.preview
        @set 'removePreviewPort', true

    apply: ->
      return if @get 'saving'
      container = @get 'model.appContainer'
      @set 'saving', true

      events = App.get 'eventsSocket'

      reconfigureComplete = =>
        @set 'saving', false
        @set 'removePreviewPort', false
        container.set 'newPreviewPort', null
        events.off 'container.reconfigured', onReconfigured
        events.off 'container.reconfigure_failed', onReconfigureFailed

      onReconfigured = (data) =>
        reconfigureComplete()
        container.setConfig(data.config)
        App.snackbar.post 'Port changes applied!'

      onReconfigureFailed = (data) =>
        reconfigureComplete()
        App.postError(data.error, "Failed to apply port changes!")

      events.on 'container.reconfigured', onReconfigured
      events.on 'container.reconfigure_failed', onReconfigureFailed

      env = Em.copy container.get('config.env')
      if newPreviewPort = container.get('newPreviewPort')
        env["NITROUS_PREVIEW_PORT"] = newPreviewPort
      if @get 'removePreviewPort'
        delete env["NITROUS_PREVIEW_PORT"]

      container.saveConfig(
        port: container.get('config.port')
        env: transformEnvHashToArray env
      ).then =>
        App.snackbar.post 'Applying port changes...'
        @set 'closeApplyDialog', true
      , (jqxhr) ->
        reconfigureComplete()
        App.postError(jqxhr.responseJSON, "Failed to apply port changes!")

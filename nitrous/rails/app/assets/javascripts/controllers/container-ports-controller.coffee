transformEnvHashToArray = (hash) ->
  envVars = []
  if hash
    for own key, value of hash
      envVars.push("#{key}=#{value}") if value
  envVars

App.ContainerPortsController = Em.ObjectController.extend
  saving: (->
    m = @get 'model'
    m && m.get('underReconfig')
  ).property 'model', 'model.underReconfig'
  port: null
  newPreviewPort: null
  setPreviewPort: false
  removePreviewPort: false
  publicPorts: null

  setPublicPorts: (->
    return if @get 'saving'
    publicPorts = Em.A()
    return publicPorts unless ports = @get 'model.config.port'

    previewPort = @get('newPreviewPort') || @get('model.previewPort')

    for port in ports
      regex = ///
        ^public:  # should be a public port binding
        (\d*):    # host port
        (\d*)     # container port
        (\/(?:tcp|udp))?  # can be followed by tcp or udp
        $         # end of string
      ///
      if m = port.match(regex)
        hostPort = m[1] || 'To be assigned'
        containerPort = parseInt(m[2])
        publicPorts.push { port: port, hostPort: hostPort, containerPort: containerPort, preview: m[1] == previewPort }
    @set 'publicPorts', publicPorts.sortBy "containerPort"
  ).observes 'saving', 'model.config.port.@each', 'model.previewPort', 'newPreviewPort'

  clear: ->
    @_super.apply this, arguments
    @setProperties
      port: null
      newPreviewPort: null
      setPreviewPort: false
      removePreviewPort: false

  actions:
    add: ->
      port = @get 'port'
      container = @get 'model'
      portMappings = container.get('config.port')
      if @get('setPreviewPort')
        @set 'newPreviewPort', port
      pb = "public::#{port}"
      unless portMappings.contains(pb)
        portMappings.pushObject(pb)
      @set 'port', null
      @set 'setPreviewPort', false

    remove: (port) ->
      pb = port.port
      container = @get 'model'
      container.get('config.port').removeObject pb
      if port.preview
        @set 'removePreviewPort', true

    apply: ->
      return if @get 'saving'
      container = @get 'model'
      prevState = @get 'state'
      @set 'state', 'reconfiguring'

      events = App.get 'eventsSocket'

      reconfigureComplete = =>
        @set 'state', prevState
        @set 'removePreviewPort', false
        @set 'newPreviewPort', null
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
      if newPreviewPort = @get('newPreviewPort')
        env["NITROUS_PREVIEW_PORT"] = newPreviewPort
      if @get 'removePreviewPort'
        delete env["NITROUS_PREVIEW_PORT"]

      container.saveConfig(
        port: container.get('config.port')
        env: transformEnvHashToArray env
      ).then =>
        App.snackbar.post 'Applying port changes...'
      , (jqxhr) ->
        reconfigureComplete()
        App.postError(jqxhr.responseJSON, "Failed to apply port changes!")

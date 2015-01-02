App.AppPortsController = Em.ObjectController.extend
  publicPorts: Em.computed.alias 'model.appContainer.publicPorts'
  saving: false
  port: null

  clear: ->
    @_super.apply this, arguments
    @setProperties
      saving: false
      port: null

  actions:
    add: ->
      port = @get 'port'
      container = @get('model.appContainer')
      portBindings = container.get('config.port')
      pb = ['public', port, port].join(':')
      unless portBindings.contains(pb)
        portBindings.pushObject(pb)
      @set 'port', null

    remove: (port) ->
      pb = ['public', port, port].join(':')
      container = @get('model.appContainer')
      container.get('config.port').removeObject pb

    apply: ->
      return if @get 'saving'
      container = @get 'model.appContainer'
      @set 'saving', true

      events = App.get 'eventsSocket'

      reconfigureComplete = =>
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

      container.saveConfig(
        port: container.get('config.port')
      ).then(=>
        App.snackbar.post 'Applying port changes...'
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Failed to apply port changes!")
      ).finally =>
        @set 'saving', false

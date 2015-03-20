hostActions =
  update:
    name: 'Update'
    progressName: 'Updating'
    description: 'This will update the host and may restart freighter and/or docker.'
    successMessage: 'Successfully updated host'
    failureMessage: 'Failed to update host!'
    perform: (host) ->
      Em.$.ajax
        type: 'POST'
        url: "/api/v0/hosts/#{host.slug}/upgrade"
        headers: { Authorization: "Bearer #{App.get 'token'}" }

  start:
    name: 'Start'
    progressName: 'Starting'
    description: 'This will start the host.'
    successMessage: 'Successfully started the host'
    failureMessage: 'Failed to start host!'
    perform: (host) ->
      Em.$.ajax
        type: 'POST'
        url: "/api/v0/hosts/#{host.slug}/start"
        headers: { Authorization: "Bearer #{App.get 'token'}" }

  stop:
    name: 'Stop'
    progressName: 'Stopping'
    description: 'This will stop the host.'
    successMessage: 'Successfully stopped the host'
    failureMessage: 'Failed to stop host!'
    perform: (host) ->
      Em.$.ajax
        type: 'POST'
        url: "/api/v0/hosts/#{host.slug}/stop"
        headers: { Authorization: "Bearer #{App.get 'token'}" }

  restart:
    name: 'Restart'
    progressName: 'Restarting'
    description: 'This will restart the host.'
    successMessage: 'Successfully restarted the host'
    failureMessage: 'Failed to restart host!'
    perform: (host) ->
      Em.$.ajax
        type: 'POST'
        url: "/api/v0/hosts/#{host.slug}/restart"
        headers: { Authorization: "Bearer #{App.get 'token'}" }

  terminate:
    name: 'Terminate'
    progressName: 'Terminating...'
    description: 'This will permanently delete the host and all services, images, volumes, and archives within the host.'
    successMessage: 'Successfully terminated host'
    failureMessage: 'Failed to terminate host!'
    perform: (host) ->
      Em.$.ajax
        type: 'DELETE'
        url: "/api/v0/hosts/#{host.slug}"
        headers: { Authorization: "Bearer #{App.get 'token'}" }

  terminateUnaccounted:
    name: 'Terminate'
    progressName: 'Terminating...'
    description: 'This will permanently delete the host and all services, images, volumes, and archives within the host.'
    successMessage: 'Successfully terminated host'
    failureMessage: 'Failed to terminate host!'
    perform: (host) ->
      hostId = host.id
      hostRegion = host.region
      ip = host.ip_address

      Em.$.ajax(
        type: 'DELETE'
        url: '/api/v0/admin/terminate_unaccounted_host'
        data:
          provider: 'amazon'
          provider_identifier: hostId
          region: hostRegion
          ip_address: ip
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).then =>
        App.snackbar.post('Unaccounted host was successfully terminated.')
        @fetchHosts()

App.AdminAmazonHostsController = Em.Controller.extend
  clear: ->
    @fetchHosts()

  fetchHosts: ->
    @set('fetchingHosts', true)

    Em.$.ajax(
      type: 'GET'
      url: '/api/v0/admin/amazon/hosts'
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    )
    .then((data) =>
      @set 'hosts', data.hosts
      @set('fetchingHosts', false)
    , =>
      @set('fetchingHosts', false)
    )

  closeDialog: ->
    @set('targetHost', null)
    @set('selectedAction', null)
    @set('inProgress', false)
    @set('availableActions', [])
    ui.hideModal('#action-modal-dialog')

  closeDialogWithMessage: (message) ->
    @fetchHosts()
    @closeDialog()
    App.snackbar.post(message)

  showActionDialog: (host, actions) ->
    @set('targetHost', host)
    @set('availableActions', actions)
    ui.showModal('#action-modal-dialog')

  actions:
    refreshHosts: ->
      @fetchHosts()

    showHealthyHostDialog: (host) ->
      if host.state == 'running'
        @showActionDialog(host, [hostActions.update, hostActions.restart, hostActions.stop, hostActions.terminate])
      else if host.state == 'stopped'
        @showActionDialog(host, [hostActions.start, hostActions.terminate])

    showProblemHostDialog: (host) ->
      @showActionDialog(host, [hostActions.terminate])

    showUnaccountedHostDialog: (host) ->
      @showActionDialog(host, [hostActions.terminateUnaccounted])

    cancelAction: ->
      @closeDialog()

    selectAction: (action) ->
      @set('selectedAction', action)

    confirmAction: ->
      host = @get('targetHost')
      action = @get('selectedAction')
      console.log('performing action ', action, ' on host', host)
      @set('inProgress', true)

      action.perform(host)
      .then(=>
        @closeDialogWithMessage(action.successMessage)
      , =>
        @closeDialogWithMessage(action.failureMessage)
      )

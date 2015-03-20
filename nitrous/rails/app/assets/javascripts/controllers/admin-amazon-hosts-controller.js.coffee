hostActions =
  update:
    name: 'Update'
    progressName: 'Updating'
    description: 'this will update and restart the host.'
    successMessage: 'Successfully updated host'
    failureMessage: 'Failed to update host!'
    perform: (host) ->
      console.log('updating host', host)
      Em.$.ajax
        type: 'POST'
        url: "/api/v0/hosts/#{host.slug}/upgrade"
        headers: { Authorization: "Bearer #{App.get 'token'}" }

  restart:
    name: 'Restart'
    progressName: 'Restarting'
    description: 'This will restart the host.'
    successMessage: 'Successfully restarted the host'
    failureMessage: 'Failed to restart host!'
    perform: (host) ->
      console.log('restarting host', host)
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
      console.log('terminating host', host)
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
      console.log('terminating unaccounted host', host)
      hostId = host.id
      hostRegion = host.region
      ip = host.ip_address

      Em.$.ajax(
        type: 'DELETE'
        url: "/api/v0/admin/terminate_unaccounted_host?provider=amazon&provider_identifier=#{hostId}&region=#{hostRegion}&ip_address=#{ip}"
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
      @showActionDialog(host, [hostActions.update, hostActions.restart, hostActions.terminate])

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
###
hostActions abstracts out different types of admin actions
we want to perform on a host. This allows us to create generic
modal dialog that shows different actions that can be performed
on selected hosts.
###
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
    perform: (host, data) ->
      Em.$.ajax
        type: 'POST'
        url: "/api/v0/hosts/#{host.slug}/restart"
        data: data
        headers: { Authorization: "Bearer #{App.get 'token'}" }

  terminate:
    name: 'Terminate'
    progressName: 'Terminating'
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
    progressName: 'Terminating'
    description: 'This will permanently delete the host and all services, images, volumes, and archives within the host.'
    successMessage: 'Successfully terminated host'
    failureMessage: 'Failed to terminate unaccounted host!'
    perform: (host) ->
      hostId = host.id
      hostRegion = host.region
      ip = host.ip_address

      Em.$.ajax
        type: 'POST'
        url: '/api/v0/admin/terminate_unaccounted_host'
        data:
          provider: 'amazon'
          provider_identifier: hostId
          region: hostRegion
          ip_address: ip
        headers: { Authorization: "Bearer #{App.get 'token'}" }

  stateChanger: (newState) ->
    name: "Update State to #{newState}"
    progressName: 'Updating state'
    description: "This will update the host db state to '#{newState}'."
    successMessage: 'Successfully updated host state'
    failureMessage: 'Failed to update host state!'
    perform: (host) ->
      Em.$.ajax
        type: 'POST'
        url: '/api/v0/admin/change_host_state'
        data:
          slug: host.slug
          old_state: host.state
          new_state: newState
        headers: { Authorization: "Bearer #{App.get 'token'}" }

Lib.HostActions =
  hostActions: hostActions

  # Actions available to a healthy host depends on
  # the current state of the host
  healthyHostActions: (host) ->
    if host.state == 'running'
      [hostActions.update, hostActions.restart, hostActions.stop, hostActions.terminate]
    else if host.state == 'stopped'
      [hostActions.start, hostActions.terminate]
    else
      [hostActions.terminate]

  inconsistentHostActions: (host) ->
    actions = [hostActions.terminate]
    if host.state != 'stopped'
      actions.unshift(hostActions.stateChanger('stopped'))
    if host.state != 'running'
      actions.unshift(hostActions.stateChanger('running'))
    return actions

  # Put other types of common collection of host actions into array
  healthyHostBulkActions: [hostActions.update, hostActions.terminate]
  inconsistentHostBulkActions: [hostActions.stateChanger('running'), hostActions.stateChanger('stopped'), hostActions.terminate]
  problemHostActions: [hostActions.terminate]
  unaccountedHostActions: [hostActions.terminateUnaccounted]

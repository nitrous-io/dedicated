#= require ../lib/host-actions

{ healthyHostActions, healthyHostBulkActions, problemHostActions, unaccountedHostActions } = Lib.HostActions

# For displaying identifier of a host
# Use host.slug if it is a DB host
# and host.id if it is a live host
getHostSlug = (host) ->
  host.slug || host.id || 'unknown-host'

###
Because our host entries are all plain objects,
we need some hack to allow the Ember checkboxes to
store the checked state in some Ember object.

The way we workaround this is to attach a uiState
Ember object on each entry to store UI states.

The entry here can be either db host, live host, or both
###
attachState = (entries) ->
  entries.forEach (entry) ->
    entry.uiState = Em.Object.create({
      selected: false
    })

attachStateToAll = (hostList) ->
  attachState(hostList.unaccounted_hosts)
  attachState(hostList.dangling_hosts)
  attachState(hostList.inconsistent_hosts)
  attachState(hostList.healthy_hosts)

# Find out which entries are selected by the checkboxes
# We don't care what type the host entry is because
# all UI states are stored at the root attribute
selectedHosts = (entries) ->
  entries.filter (entry) ->
    entry.uiState.get('selected')

# Used for double db+live entries. After filtering out
# the selected hosts we extract out the db_host field
# to get the DB host entry
selectedDbHosts = (entries) ->
  selectedHosts(entries).map (entry) ->
    entry.db_host

App.AdminAmazonHostsController = Em.Controller.extend
  showFixBorn: false
  hostbuilderLog: null
  saving: false

  clear: ->
    @refreshHosts()
    @set 'showFixBorn', false
    @set 'hostbuilderLog', null
    @set 'saving', false

  # Fetch the latest host list from the server.
  # This will uncheck all checkbox states.
  refreshHosts: ->
    @set('fetchingHosts', true)

    Em.$.ajax(
      type: 'GET'
      url: '/api/v0/admin/amazon/hosts'
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    )
    .then((data) =>
      hosts = data.hosts
      attachStateToAll(hosts)

      @set('hosts', hosts)
      @set('fetchingHosts', false)
    , =>
      @set('fetchingHosts', false)
    )

  # Use for displaying list of selected hosts
  # concatenated into a comma-separated string.
  targetHostSlugs: (->
    @get('targetHosts')?.map(getHostSlug).join(', ')
  ).property 'targetHosts'

  closeDialog: ->
    @set('targetHosts', [])
    @set('selectedAction', null)
    @set('availableActions', [])
    @set('showFixBorn', false)
    @set('hostbuilderLog', null)
    @set('saving', false)
    ui.hideModal('#action-modal-dialog')

  # Generic way of showing an action dialog
  # with available host actions to perform on provided hosts.
  showActionDialog: (hosts, actions) ->
    if hosts.length == 0
      return

    @set('targetHosts', hosts)
    @set('availableActions', actions)
    ui.showModal('#action-modal-dialog')

  # Perform a host action on selected host.
  # Always succeeds and action failure will
  # be posted to the snackbar and then ignored.
  performActionOnHost: (action, host) ->
    App.snackbar.post("#{action.progressName} #{getHostSlug(host)}...")

    action.perform(host)
    .then(=>
      action.successMessage
    , =>
      action.failureMessage
    ).then((message) =>
      App.snackbar.post(message)
      @refreshHosts()
    )

  # Bulk perform a host action on list of selected hosts.
  # The action is performed in parallel to all hosts.
  # After all actions completed a snackbar is posted to
  # inform the completion
  performActionOnHosts: (action, hosts) ->
    if hosts.length == 1
      return @performActionOnHost(action, hosts[0])

    Em.RSVP.Promise.all(hosts.map (host) =>
      @performActionOnHost(action, host)
    ).then =>
      App.snackbar.post('Action have been performed on all selected hosts')

  actions:
    refreshHosts: ->
      @refreshHosts()

    showHealthyHostDialog: (host) ->
      @showActionDialog([host], healthyHostActions(host))

    showProblemHostDialog: (host) ->
      @set('showFixBorn', true)
      @showActionDialog([host], problemHostActions)

    showUnaccountedHostDialog: (host) ->
      @showActionDialog([host], unaccountedHostActions)

    showHealthyHostBulkDialog: ->
      hosts = @get('hosts').healthy_hosts
      selected = selectedDbHosts(hosts)
      @showActionDialog(selected, healthyHostBulkActions)

    showInconsistentHostBulkDialog: ->
      hosts = @get('hosts').inconsistent_hosts
      selected = selectedDbHosts(hosts)
      @showActionDialog(selected, problemHostActions)

    showDanglingHostBulkDialog: ->
      hosts = @get('hosts').dangling_hosts
      selected = selectedHosts(hosts)
      @showActionDialog(selected, problemHostActions)

    showUnaccountedHostBulkDialog: ->
      hosts = @get('hosts').unaccounted_hosts
      selected = selectedHosts(hosts)
      @showActionDialog(selected, unaccountedHostActions)

    cancelAction: ->
      @closeDialog()

    selectAction: (action) ->
      @set('selectedAction', action)

    confirmAction: ->
      action = @get('selectedAction')
      hosts = @get('targetHosts')
      @closeDialog()
      @refreshHosts()
      @performActionOnHosts(action, hosts)

    fixBorn: ->
      return unless @get 'hostbuilderLog'
      host = @get 'targetHosts.firstObject'
      @set 'saving', true
      Em.$.ajax(
        type: 'POST'
        data: { host_builder: @get('hostbuilderLog') }
        url: "/api/v0/admin/amazon/fix_born/#{host.id}"
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).done((data, status, jqxhr) =>
        @closeDialog()
        @refreshHosts()
      ).fail (jqxhr) =>
        @set 'saving', false
        App.postError(jqxhr.responseJSON, 'Failed to fix born!')

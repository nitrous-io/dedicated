#= require ./base-model

App.Box = klass = App.BaseModel.extend
  name: Em.attr()
  host: Em.attr()
  state: Em.attr()
  memory: Em.attr(Number)
  storage: Em.attr(Number)
  cpu_multiplier: Em.attr(Number)
  virtual_memory: Em.attr(Number)
  n2o_cost: Em.attr(Number)
  expected_cpu_multiplier: Em.attr(Number)
  expected_virtual_memory: Em.attr(Number)
  version: Em.attr()
  box_template_id: Em.attr()
  owner_id: Em.attr()
  region_id: Em.attr()
  port: Em.attr(Number)
  source_snapshot_id: Em.attr()
  box_server_uri: Em.attr()
  container_name: Em.attr()
  auth_username: Em.attr()
  auth_password: Em.attr()
  backend_available: Em.attr()
  force_shutdown: Em.attr()
  custom_domain: Em.attr()
  custom_domains_allowed: Em.attr()
  last_added_repository_id: Em.attr()
  repository: Em.attr()

  memory_usage: null
  storage_usage: null

  public_key: null
  publicKeyInfo: null


  lastBoxUpdatedTimestamp: null # to ignore late-delivered aio:box_updated shover event

  validations:
    name: (v) -> "can't be blank" unless v && v.length > 0
    region_id: (v) -> "needs to be selected" unless v

  expanded: false
  loading: false # when box control request is happening
  loadingPublicKey: false # when loading public key from server
  addingPublicKeyToGitHub: false # when adding public key to github

  vmem_multiplier: (-> # no camel case to be consistent with other properties
    @_formatMemoryMultiplier(@get('virtual_memory'), @get('memory'))
  ).property 'virtual_memory', 'memory'

  expected_vmem_multiplier: (-> # no camel case to be consistent with other properties
    @_formatMemoryMultiplier(@get('expected_virtual_memory'), @get('memory'))
  ).property 'virtual_memory', 'memory'

  hasBaseCPUShare: (->
    parseInt(@get('cpu_multiplier'), 10) == 1
  ).property 'cpu_multiplier'

  requiresCPUShareDowngrade: (->
    @get('expected_cpu_multiplier') < @get('cpu_multiplier')
  ).property 'cpu_multiplier', 'expected_cpu_multiplier'

  requiresCPUShareUpgrade: (->
    @get('expected_cpu_multiplier') > @get('cpu_multiplier')
  ).property 'cpu_multiplier', 'expected_cpu_multiplier'

  requiresVirtualMemoryDowngrade: (->
    @get('expected_virtual_memory') < @get('virtual_memory')
  ).property 'virtual_memory', 'expected_virtual_memory'

  requiresVirtualMemoryUpgrade: (->
    @get('expected_virtual_memory') > @get('virtual_memory')
  ).property 'virtual_memory', 'expected_virtual_memory'

  requiresDowngrade: (->
    @get('requiresCPUShareDowngrade') || @get('requiresVirtualMemoryDowngrade')
  ).property 'requiresCPUShareDowngrade', 'requiresVirtualMemoryDowngrade'

  requiresUpgrade: (->
    @get('requiresCPUShareUpgrade') || @get('requiresVirtualMemoryUpgrade')
  ).property 'requiresCPUShareUpgrade', 'requiresVirtualMemoryUpgrade'

  ownBox: (->
    if currentUserId = App.get('currentUser.id')
      currentUserId == @get('owner_id')
  ).property 'App.currentUser.id'

  notOwnBox: (->
    !@get('ownBox')
  ).property 'ownBox'

  sshURI: (->
    port = @get 'port'
    host = @get 'host'
    if host && port
      "ssh://action@#{@get 'host'}:#{@get 'port'}"
  ).property 'host', 'port'

  boxHost: (->
    name = @get 'container_name'
    host = @get 'host'
    if name && host
      "#{name}.#{host}"
  ).property 'name', 'host'

  region: (->
    App.Region.findById @get('region_id')
  ).property 'region_id'

  boxTemplate: (->
    App.BoxTemplate.findById @get('box_template_id')
  ).property 'box_template_id'

  sourceSnapshot: (->
    App.Snapshot.findById @get('source_snapshot_id')
  ).property 'source_snapshot_id'

  snapshots: (->
    App.Snapshot.Collection.create
      content: App.get('snapshots').filter (snapshot) =>
        @get('id') == snapshot.get('box_id')
  ).property 'App.snapshots.@each'

  snapshottable: (->
    @get('state') == 'stopped'
  ).property 'state'

  webTerminalPath: (->
    "#{@constructor.url}/#{@get 'id'}/terminal"
  ).property 'id'

  stopped: (->
    @get('state') == 'stopped'
  ).property 'state'

  running: (->
    @get('state') == 'running'
  ).property 'state'

  terminated: (->
    @get('state') == 'terminated'
  ).property 'state'

  provisioning: (->
    @get('state') == 'provisioning'
  ).property 'state'

  snapshotting: (->
    @get('state') == 'snapshotting'
  ).property 'state'

  failed: (->
    @get('state') == 'failed'
  ).property 'state'

  working: (->
    !(_.include(['stopped', 'running', 'terminated', 'provisioning', 'failed'], @get('state')))
  ).property 'state'

  underMaintenance: (->
    @get('state') == "migrating" || !@get('region.enabled')
  ).property 'state', 'region.enabled'

  stateForOrb: (->
    if @get 'underMaintenance'
      'under_maintenance'
    else
      @get 'state'
  ).property 'state', 'underMaintenance'

  stateForDisplay: (->
    switch state = @get 'state'
      when 'failed'
        'provisioning failed'
      when 'changing_specs'
        'changing specs'
      when 'cloning_repo'
        'downloading repository'
      else
        state
  ).property 'state'

  customDomainPreviewable: (->
    @get('custom_domains_allowed') && @get('custom_domain')
  ).property 'custom_domains_allowed', 'custom_domain'

  makeBoxControlRequest: (action, method='PUT', params) ->
    @set 'loading', true
    promise = @makeRequest action, method, params
    promise.then (json) =>
      json = json[@constructor.rootKey]
      @loadHash json
      @set 'loading', false
    , =>
      @set 'loading', false
    promise

  start: ->
    @makeBoxControlRequest 'start'

  stop: ->
    @makeBoxControlRequest 'stop'

  restart: ->
    @makeBoxControlRequest 'restart'

  provision: ->
    @makeBoxControlRequest 'provision'

  terminate: (opts={}) ->
    snapshot = opts.snapshot ? false
    @makeBoxControlRequest null, 'DELETE', snapshot:snapshot

  cloneRepo: (repo, callback) ->
    @set 'loading', true
    data = box:
      repository_clone_url: repo.get('clone_url')
    $.ajax(
      type: 'PUT'
      dataType: 'json'
      data: data
      url: "#{@get 'resourceUrl'}/#{@get 'id'}/clone_repo/#{repo.id}"
    ).always(=>
      @set 'loading', false
    ).done (json) =>
      json = json[@get 'resourceName']
      @deserialize json
      callback(null)
    .fail (e) =>
      if e.status == 422
        errors = @get 'errors' || {}
        errors = $.parseJSON(e.responseText).errors
        @set 'errors', errors
        callback(errors)

  readDefaultPublicKey: ->
    @set 'publicKeyInfo', null
    @set 'loadingPublicKey', true
    @makeRequest('default_public_key', 'GET').then =>
      @set 'loadingPublicKey', false
    , =>
      @set 'publicKeyInfo', 'Failed to fetch public key.'
      @set 'loadingPublicKey', false

  addPublicKeyToGitHub: ->
    @set 'publicKeyInfo', null
    @set 'addingPublicKeyToGitHub', true
    @makeRequest('add_public_key_to_github', 'POST').then =>
      @set 'addingPublicKeyToGitHub', false
    , =>
      @set 'publicKeyInfo', 'Failed to add key to GitHub.'
      @set 'addingPublicKeyToGitHub', false

  fetchBoxInfo: ->
    @makeRequest 'info'

  removeCollaborator: (username, callback) ->
    @set 'loading', true

    @makeRequest("collaborators/#{username}", 'DELETE').then =>
      @set 'loading', false
      callback(true)
    , =>
      @set 'loading', false

  updateAuth: (cb) ->
    params = box:
      auth_username: @get('auth_username')
      auth_password: @get('auth_password')

    @makeBoxControlRequest('auth', 'PUT', params).then =>
      cb null
    , =>
      cb new Error(@get 'errors')

  updateCustomDomain: (cb) ->
    params = box:
      custom_domain: @get('custom_domain')

    @makeBoxControlRequest('custom_domain', 'PUT', params).then =>
      cb null
    , =>
      cb new Error(@get 'errors')

  _formatMemoryMultiplier: (number, base) ->
    if number % base == 0
      number / base
    else
      parseFloat((number / base).toFixed(1))

klass.reopenClass
  rootKey: 'box'
  collectionKey: 'boxes'
  url: '/boxes'

  minBoxN2O: ->
    config = App.get 'config'
    config.get('minMemoryMB') / 128 * config.get('n2o128mbMemory') + config.get('minStorageMB') / 250 * config.get('n2o250mbStorage')

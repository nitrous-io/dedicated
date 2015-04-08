#= require ./model
attr = SM.attr

App.Container = klass = App.Model.extend
  id: attr('number', serialize: false)
  cid: attr(serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  image: attr()
  template: attr()
  slug: attr()
  name: attr()
  state: attr(serialize: false)
  cmd: attr('array', serialize: false)
  entrypoint: attr('array', serialize: false)
  exitCode: attr('number', as: 'exit_code', serialize: false)
  previewHostname: attr(as: 'preview_hostname', serialize: false)
  ipAddress: attr(as: 'ip_address', serialize: false)
  gatewayIP: attr(as: 'gateway_ip', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)
  startedAt: attr('date', as: 'started_at', serialize: false)
  finishedAt: attr('date', as: 'finished_at', serialize: false)
  terminatedAt: attr('date', as: 'terminated_at', serialize: false)

  shared: false

  config: (->
    return unless @get('id') # Load config only after container is saved
    config = Em.ObjectProxy.create()
    @loadConfig()
    config
  ).property 'id'

  collaborators: (->
    if id = @get('id')
      App.ContainerCollaborator.fetch(containerId: id).value
  ).property 'id'

  underReconfig: attr('boolean', serialize: false)

  isRunning: Em.computed.equal 'state', 'running'
  isStopped: Em.computed.equal 'state', 'stopped'
  isTerminated: Em.computed.equal 'state', 'terminated'

  isNitrousImage: Em.computed.notEmpty 'config.env.NITROUS_IMAGE_VERSION'

  hostnameBinding: Em.Binding.oneWay 'host.hostname'
  loginUsernameBinding: Em.Binding.oneWay 'config.env.NITROUS_USERNAME'
  previewPortBinding: Em.Binding.oneWay 'config.env.NITROUS_PREVIEW_PORT'
  freighterdUrlBinding: Em.Binding.oneWay 'host.freighterdUrl'

  sshEnabled: Em.computed.equal 'config.env.NITROUS_SSH_ENABLED', 'true'

  sshPort: (->
    return unless portMappings = @get 'config.port'
    prefix = 'public:'
    suffices = [':22/tcp', ':22']
    return unless sshPort = portMappings.find (port, i) ->
      port.startsWith(prefix) && (port.endsWith(suffices[0]) || port.endsWith(suffices[1]))
    sshPort.match(/^public:(\d+):22(\/tcp)?$/)?[1]
  ).property 'config.port'

  sshURL: (->
    return unless @get('sshEnabled') && @get('sshPort') && @get('sshKeysAdded')

    user = @get('loginUsername') || 'root'
    hostname = @get 'hostname'
    sshPort = @get 'sshPort'
    "ssh://#{user}@#{hostname}:#{sshPort}"
  ).property 'sshEnabled', 'sshKeysAdded', 'loginUsername', 'hostname', 'sshPort'

  sshKeysAdded: (->
    sshKeys = @get('userKeys')
    sshKeys?.content?.length > 0
  ).property 'userKeys', 'userKeys.@each'

  previewURL: (->
    host = @get 'previewHostname'
    port = @get('previewPort') || 3000
    return unless host
    url = "http://#{host}"
    url += ":#{port}" if port && port != 3000
  ).property 'previewHostname', 'previewPort'

  ideURL: (->
    "/ide/#/#{@get 'slug'}"
  ).property 'slug'

  displayName: (->
    @get('name') || @get('slug')
  ).property 'slug'

  exitCodeString: (->
   if (code = @get 'exitCode')?
     code.toString()
  ).property 'exitCode'

  selected: (->
    App.get('selectedContainer') == this
  ).property 'App.selectedContainer'

  host: (->
    if hostId = @get 'hostId'
      App.get('hosts')?.findBy('id', hostId)
  ).property 'hostId', 'App.hosts.@each'

  userKeys: (->
    if id = @get('id')
      App.UserKey.fetch(containerId: id).value
  ).property 'id'

  containerKeys: (->
    if id = @get('id')
      App.ContainerKey.fetch(containerId: id).value
  ).property 'id'

  eventOccurred: (evt) ->
    if evt in ['container.started', 'container.stopped', 'container.restarted', 'container.updated', 'container.reconfigured', 'container.reconfigure_failed']
      @set 'underReconfig', false

  prefetchResources: ->
    @loadConfig()
    @get 'collaborators'

  loadConfig: ->
    @ajax('GET', 'config').then (json) =>
      @setConfig(json.config)

  setConfig: (jsonConfig) ->
    config =
      env: transformEnvArrayToHash(jsonConfig.env)
      port: jsonConfig.port || []
      volume: jsonConfig.volume
      privileged: jsonConfig.privileged
      publish_all: jsonConfig.publish_all
    @set 'config.content', config
    config

  saveConfig: (config)  ->
    @set 'underReconfig', true
    @ajax 'POST', 'reconfig',
      env: config.env
      volume: config.volume
      port: config.port || []
      privileged: config.privileged
      publish_all: config.publish_all
      async: true

  requestToken: (permissions) ->
    @ajax 'POST', 'token', permission: permissions

klass.reopenClass
  primaryKey: 'slug'
  url: '/containers'
  rootKey: 'container'
  collectionKey: 'containers'

transformEnvArrayToHash = (envVars) ->
  hash = {}
  return hash unless envVars
  for v in envVars
    m = v.match /^([^=]+)=(.*)$/
    key = m[1]
    val = m[2]
    hash[key] = val
  hash

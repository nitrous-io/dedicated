#= require ./model
attr = SM.attr

App.Container = klass = App.Model.extend
  id: attr('number', serialize: false)
  cid: attr(serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  image: attr()
  slug: attr()
  name: attr()
  state: attr(serialize: false)
  cmd: attr('array', serialize: false)
  entrypoint: attr('array', serialize: false)
  exitCode: attr('number', as: 'exit_code', serialize: false)
  ipAddress: attr(as: 'ip_address', serialize: false)
  gatewayIP: attr(as: 'gateway_ip', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)
  startedAt: attr('date', as: 'started_at', serialize: false)
  finishedAt: attr('date', as: 'finished_at', serialize: false)
  terminatedAt: attr('date', as: 'terminated_at', serialize: false)

  config: null

  isRunning: Em.computed.equal 'state', 'running'
  isStopped: Em.computed.equal 'state', 'stopped'
  isTerminated: Em.computed.equal 'state', 'terminated'

  hostnameBinding: Em.Binding.oneWay 'host.hostname'
  loginUsernameBinding: Em.Binding.oneWay 'config.env.NITROUS_USERNAME'
  previewPortBinding: Em.Binding.oneWay 'config.env.NITROUS_PREVIEW_PORT'

  sshEnabled: Em.computed.equal 'config.env.NITROUS_SSH_ENABLED', 'true'

  sshPort: (->
    return unless portBindings = @get 'config.port'
    for port in portBindings
      for suffix in [':22/tcp', ':22']
        suffixIdx = port.indexOf(suffix)
        if port.indexOf('public:') == 0 && suffixIdx != -1 && suffixIdx == (port.length - suffix.length)
          return port.match(/^public:(\d+):22(\/tcp)?$/)[1]
  ).property 'config.port'

  sshURL: (->
    return unless @get('sshEnabled') && @get('sshPort')
    user = @get 'loginUsername'
    hostname = @get 'hostname'
    sshPort = @get 'sshPort'
    "ssh://#{user}@#{hostname}:#{sshPort}"
  ).property 'sshEnabled', 'loginUsername', 'hostname', 'sshPort'

  previewURL: (->
    hostname = @get 'hostname'
    previewPort = @get 'previewPort'
    if hostname && previewPort
      "http://#{hostname}:#{previewPort}/"
  ).property 'hostname', 'previewPort'

  ideURL: (->
    location.href.replace(location.pathname+location.hash, '/ide')
  ).property()

  displayName: (->
    @get('name') || @get('slug')
  ).property 'name', 'slug'

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

  keys: (->
    if id = @get('id')
      App.PublicKey.fetch(containerId: id).value
  ).property 'id'

  loadConfig: ->
    new Em.RSVP.Promise (resolve, reject) =>
      @ajax('GET', 'config').then (json) =>
        config = @setConfig(json.config)
        resolve(config)
      , (jqxhr) ->
        reject(jqxhr)

  setConfig: (jsonConfig) ->
    config =
      env: transformEnvArrayToHash(jsonConfig.env)
      port: jsonConfig.port
      volume: jsonConfig.volume
      privileged: jsonConfig.privileged
      publish_all: jsonConfig.publish_all
    @set 'config', config
    config

  saveConfig: (config)  ->
    @ajax 'POST', 'reconfig',
      env: config.env
      volume: config.volume
      port: config.port
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

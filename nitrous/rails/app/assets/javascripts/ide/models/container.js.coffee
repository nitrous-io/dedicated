PREVIEW_PORT_START = 1024
SUPPORTED_SYSTEM_PORTS = [80]

Promise = Em.RSVP.Promise

App.Container = klass = Em.Object.extend
  id: null
  user_id: null
  host_id: null
  slug: null
  name: null
  state: null
  preview_hostname: null

  config: null
  host: null

  metadata: null

  env: (->
    transformEnvArrayToHash @get('config').env
  ).property 'config'

  loginUsername: (->
    u = @get('env')["NITROUS_USERNAME"] || @get('user')
    if u == ""
      return "root"
    u
  ).property 'env'

  previewPort: (->
    p = @get('env')["NITROUS_PREVIEW_PORT"]
    if p
      parseInt(p)
    else
      null
  ).property 'env'

  freighterdUrl: (->
    hostname = @get('host')?.proxy_hostname
    port = @get('host')?.freighterd_port
    if hostname
      if port && port != 65333
        return "#{hostname}:#{port}"
      hostname
  ).property 'host'

  homePath: (->
    username = @get 'loginUsername'
    if username == "root"
      "/"
    else
      "/home/#{username}"
  ).property 'loginUsername'

  previewablePorts: (->
    publicPorts = Em.A()
    previewPort = @get('previewPort')

    return publicPorts unless @get('config.port')
    for port in @get('config.port')
      regex = ///
        ^public:  # should be a public port binding
        (\d*):    # host port
        (\d*)     # container port
        (\/tcp)?  # can be followed by tcp
        $         # end of string
      ///
      if m = port.match(regex)
        hostPort = m[1]
        containerPort = parseInt(m[2])
        if @validPreviewPort(containerPort)
          publicPorts.push { port: port, hostPort: hostPort, containerPort: containerPort, preview: containerPort == @get('previewPort') }
    publicPorts.sortBy('containerPort')
  ).property 'config', 'config.port.@each'

  validPreviewPort: (port) ->
    port == 80 || port >= 1024

  requestToken: (permissions) ->
    Promise.resolve Em.$.ajax
      type: 'POST',
      url: "/api/v0/containers/#{@get 'slug'}/token",
      headers: { Authorization: "Bearer #{App.get 'token'}" }
      data: {
        permission: permissions
      }

  loadHost: ->
    Promise.resolve Em.$.ajax
      type: 'GET',
      url: "/api/v0/hosts/#{@get 'host_id'}",
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    .then (data) =>
      @set 'host', data.host
      data

  setMetadata: (data)->
    Promise.resolve Em.$.ajax
      type: 'PUT',
      url: "/api/v0/containers/#{@get 'slug'}/metadata",
      headers: { Authorization: "Bearer #{App.get 'token'}" },
      data: data
    .then (retData) =>
      Object.keys(data).forEach (k) =>
        @set("metadata.#{k}", data[k])
      retData

  # Get freighter info for new feature detection
  # Assume empty feature { } on error retrieving
  # because old freighter server would return 404
  getFreighterInfo: ->
    freighterdUrl = @get 'freighterdUrl'
    url = "https://#{freighterdUrl}/freighter/info"
    Promise.resolve Em.$.ajax
      type: 'GET',
      url: url
    .then (result) =>
      result.freighter_info || { }
    .catch (err) =>
      { }

klass.reopenClass
  fetch: (slug) ->
    Promise.resolve Em.$.ajax
      type: 'GET',
      url: "/api/v0/containers/#{slug}",
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    .then (data) ->
      container = App.Container.create(data.container)
      container

transformEnvArrayToHash = (envVars) ->
  hash = {}
  return hash unless envVars
  for v in envVars
    m = v.match /^([^=]+)=(.*)$/
    key = m[1]
    val = m[2]
    hash[key] = val
  hash

App.Container = klass = Em.Object.extend
  id: null
  user_id: null
  host_id: null
  slug: null
  name: null
  state: null

  config: null
  host: null

  env: (->
    transformEnvArrayToHash @get('config').env
  ).property 'config'

  loginUsername: (->
    @get('env')["NITROUS_USERNAME"]
  ).property 'env'

  previewPort: (->
    @get('env')["NITROUS_PREVIEW_PORT"]
  ).property 'env'

  previewURL: (->
    hostname = @get('host')?.hostname
    previewPort = @get 'previewPort'
    if hostname && previewPort
      "http://#{hostname}:#{previewPort}/"
  ).property 'host', 'previewPort'

  freighterdUrl: (->
    hostname = @get('host')?.proxy_hostname
    port = @get('host')?.freighterd_port
    if hostname
      if port
        return "#{hostname}:#{port}"
      hostname
  ).property 'host'

  homePath: (->
    "/home/#{@get 'loginUsername'}"
  ).property 'loginUsername'

  requestToken: (permissions) ->
    promise = new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'POST',
        url: "/api/v0/containers/#{@get 'slug'}/token",
        headers: { Authorization: "Bearer #{App.get 'token'}" }
        data: {
          permission: permissions
        }
      ).done((data, status, jqxhr) ->
        resolve(data)
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise

  loadHost: ->
    promise = new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'GET',
        url: "/api/v0/hosts/#{@get 'host_id'}",
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).done((data, status, jqxhr) ->
        resolve(data)
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise

klass.reopenClass
  fetch: (slug) ->
    promise = new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'GET',
        url: "/api/v0/containers/#{slug}",
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).done((data, status, jqxhr) ->
        container = App.Container.create(data.container)
        resolve(container)
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise

transformEnvArrayToHash = (envVars) ->
  hash = {}
  return hash unless envVars
  for v in envVars
    m = v.match /^([^=]+)=(.*)$/
    key = m[1]
    val = m[2]
    hash[key] = val
  hash

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

  isRunning: Em.computed.equal 'state', 'running'
  isStopped: Em.computed.equal 'state', 'stopped'
  isTerminated: Em.computed.equal 'state', 'terminated'

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

  requestToken: (permissions) ->
    @ajax 'POST', 'token', permission: permissions

  requestShell: (command, cols, rows) ->
    freighterdUrl = @get 'host.freighterdUrl'
    cid = @get 'cid'
    url = "#{location.protocol}//#{freighterdUrl}/containers/#{cid}/shells"
    new Ember.RSVP.Promise (resolve, reject) =>
      @requestToken(['shell']).then (data) ->
        token = data.token
        Em.$.ajax(
          type: 'POST'
          url: url
          data:
            command: command
            width: cols
            height: rows
          headers:
            Authorization: "Token #{token}"
        ).then (shell) ->
          resolve(id: shell.id, token: token)
        , (jqxhr) ->
          reject(jqxhr)
      , (jqxhr) ->
        reject(jqxhr)

  resizeShell: (shellId, cols, rows) ->
    freighterdUrl = @get 'host.freighterdUrl'
    cid = @get 'cid'
    url = "#{location.protocol}//#{freighterdUrl}/containers/#{cid}/shells/#{shellId}/resize"
    @requestToken(['shell']).then (data) ->
      token = data.token
      Em.$.ajax
        type: 'POST'
        url: url
        data:
          width: cols
          height: rows
        headers:
          Authorization: "Token #{token}"

klass.reopenClass
  primaryKey: 'slug'
  url: '/containers'
  rootKey: 'container'
  collectionKey: 'containers'

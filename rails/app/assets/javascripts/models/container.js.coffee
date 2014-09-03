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
  ipAddress: attr(key: 'ip_address', serialize: false)
  gatewayIP: attr(key: 'gateway_ip', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)
  startedAt: attr('date', as: 'started_at', serialize: false)
  finishedAt: attr('date', as: 'finished_at', serialize: false)
  terminatedAt: attr('date', as: 'terminated_at', serialize: false)

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

klass.reopenClass
  primaryKey: 'slug'
  url: '/containers'
  rootKey: 'container'
  collectionKey: 'containers'

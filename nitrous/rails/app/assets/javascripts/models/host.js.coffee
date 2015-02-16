#= require ./model
attr = SM.attr

App.Host = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  regionId: attr('number', as: 'region_id', serialize: false)
  planId: attr('number', as: 'plan_id', serialize: false)
  name: attr()
  slug: attr()
  state: attr(serialize: false)
  ipAddress: attr(as: 'ip_address', serialize: false)
  hostname: attr(serialize: false)
  proxyHostname: attr(as: 'proxy_hostname', serialize: false)
  freighterdPort: attr('number', as: 'freighterd_port', serialize: false)
  providerIdentifier: attr(as: 'provider_identifier', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)
  terminatedAt: attr('date', as: 'terminated_at', serialize: false)
  liveFreighterdVersion: attr(as: 'live_freighterd_version', serialize: false)
  liveDockerVersion: attr(as: 'live_docker_version', serialize: false)
  upgradeAvailable: attr('boolean', as: 'upgrade_available', serialize: false)

  # only used in create
  planSlug: attr(as: 'size')
  regionSlug: attr(as: 'region')

  stats: null
  liveHost: null

  isRunning: Em.computed.equal 'state', 'running'
  isTerminated: Em.computed.equal 'state', 'terminated'
  isOrphan: Em.computed.equal 'state', 'orphan'

  displayName: (->
    @get('name') || @get('slug')
  ).property 'slug'

  containers: (->
    if id = @get('id')
      App.get('containers')?.filterBy('hostId', id)
  ).property 'id', 'App.containers.@each'

  sortedContainers: (->
    @get('containers').sortBy('slug')
  ).property 'containers'

  registries: (->
    if id = @get('id')
      App.Registry.fetch(hostId: id).value
  ).property 'id'

  volumes: (->
    if id = @get('id')
      App.Volume.fetch(hostId: id).value
  ).property 'id'

  images: (->
    if id = @get('id')
      App.Image.fetch(hostId: id).value
  ).property 'id'

  region: (->
    if regionId = @get('regionId')
      App.get('regions').findBy('id', regionId)
  ).property 'regionId', 'App.regions.@each'

  plan: (->
    if planId = @get('planId')
      App.get('plans').findBy('id', planId)
  ).property 'planId', 'App.plans.@each'

  freighterdUrl: (->
    hostname = @get 'proxyHostname'
    port = @get 'freighterdPort'
    if hostname
      if port && port != 65333
        return "#{hostname}:#{port}"
      hostname
  ).property 'proxyHostname', 'freighterdPort'

  selected: (->
    App.get('selectedHost') == this
  ).property 'App.selectedHost'

  hostStateDidChange: (->
    if @get('state') == 'running'
      Em.run.later(this, ->
        @fetchStats()
      , 1000)
    else
      @set 'stats', null
  ).observes 'state'

  prefetchResources: ->
    @get 'images'
    @get 'registries'
    @get 'volumes'
    true

  fetchStats: ->
    if @get('state') == 'running'
      @ajax('GET', 'stats', period: '1s').then (data) =>
        if stats = data.stats
          @set 'stats', App.HostStats.create(stats)

klass.reopenClass
  url: '/hosts'
  primaryKey: 'slug'
  rootKey: 'host'
  collectionKey: 'hosts'

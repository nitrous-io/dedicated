#= require ./model
attr = SM.attr

FETCH_STATS_RETRY_COUNT = 10
FETCH_STATS_RETRY = 3000 # 3s. After a fetchStats failure, retry in..
FETCH_STATS_NEXT = 60000 # 60s. After FETCH_STATS_RETRY_COUNT has been hit, schedule another fetchStats in
HAS_ERROR_THRESHOLD = 2 # Show the initial "has error" message after X failures
CONTACT_SUPPORT_THRESHOLD = 30 # Show the contact support message after X failures

App.Host = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  regionId: attr('number', as: 'region_id', serialize: false)
  planId: attr('number', as: 'plan_id', serialize: false)
  orgId: attr('number', as: 'org_id', serialize: false)
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
  lastUpdatedAt: attr('date', as: 'last_updated_at', serialize: false)
  lastRunningAt: attr('date', as: 'last_running_at', serialize: false)
  pendingRespec: attr('boolean', as: 'pending_respec', serialize: false)
  subscriptionSlug: attr('string', as: 'subscription_slug', serialize: false)
  free: attr('boolean', serialize: false)

  # only used in create
  planSlug: attr(as: 'size')
  regionSlug: attr(as: 'region')
  orgSlug: attr(as: 'org')

  stats: null
  liveHost: null

  # True when there is an error connecting to the host.
  connectivityErrorCount: 0

  isRunning: Em.computed.equal 'state', 'running'
  isStopped: Em.computed.equal 'state', 'stopped'
  isTerminated: Em.computed.equal 'state', 'terminated'
  isOrphan: Em.computed.equal 'state', 'orphan'
  isUpdating: Em.computed.equal 'state', 'updating'
  isRespeccing: Em.computed.equal 'state', 'respeccing'

  displayName: (->
    @get('name') || @get('slug')
  ).property 'name', 'slug'

  stateForDisplay: (->
    switch state = @get('state')
      when 'respeccing'
        'upgrading in progress'
      when 'respec_failed'
        'upgrading failed'
      else
        state
  ).property 'state'

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
    return unless @get('isRunning')
    if id = @get('id')
      App.Image.fetch(hostId: id).value
  ).property 'id', 'isRunning'

  announcements: (->
    if id = @get('id')
      App.Announcement.fetch(hostId: id).value
  ).property 'id'

  topAnnouncement: (->
    if an = @get('announcements')
      an.sortBy('createdAt').reverse()[0]
  ).property 'announcements.@each'

  region: (->
    if regionId = @get('regionId')
      App.get('regionIndex')[regionId]
  ).property 'regionId', 'App.regionIndex'

  plan: (->
    if planId = @get('planId')
      App.get('plans').findBy('id', planId)
  ).property 'planId', 'App.plans.@each'

  subscription: (->
    if subscriptionSlug = @get('subscriptionSlug')
      App.get('subscriptions').findBy('slug', subscriptionSlug)
  ).property 'subscriptionSlug', 'App.subscriptions.@each'

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
    if @get('state') != 'running'
      @set 'stats', null
  ).observes 'state'

  connectivityErrorCountDidChange: (->
    if @get('connectivityErrorCount') == 0
      App.get('containers')?.reload()
  ).observes 'connectivityErrorCount'

  hasContainerCreating: (->
    App.get('hostCreatingContainer') == @get 'slug'
  ).property 'App.hostCreatingContainer'

  hasError: (->
    return false unless @get('isRunning')
    @get('connectivityErrorCount') >= HAS_ERROR_THRESHOLD
  ).property 'connectivityErrorCount', 'isUpdating'

  shouldContactSupport: (->
    @get('connectivityErrorCount') >= CONTACT_SUPPORT_THRESHOLD
  ).property 'connectivityErrorCount'

  canSafeStop: (->
    return false if @get('isUpdating')
    return @get('containers').every (con)->
      con.get('isStopped') || con.get('isTerminated')
  ).property 'containers.@each.state'

  canCancelSubscription: (->
    return false unless @get 'subscriptionSlug'
    !@get('free')
  ).property 'subscriptionSlug'

  prefetchResources: ->
    @get 'images'
    @get 'registries'
    @get 'volumes'
    if (an = @get 'announcements') && an.get('loaded')
      an.reload()
    else
      @get 'announcements'
    true

  fetchStats: (retryCount = FETCH_STATS_RETRY_COUNT) ->
    @getStats()?.then( =>
      # we successfully got through, kill all timeouts
      if @fetchStatsRetryTimeout
        window.clearTimeout @fetchStatsRetryTimeout
        @fetchStatsRetryTimeout = null

      if @fetchStatsRescheduleTimeout
        window.clearTimeout @fetchStatsRescheduleTimeout
        @fetchStatsRescheduleTimeout = null

    , =>
      # failure, handle case
      if retryCount == 0
        # reschedule a check in a minute
        # this prevents the user from needing to do any "reloading"
        return if @fetchStatsRescheduleTimeout
        @fetchStatsRescheduleTimeout = window.setTimeout =>
          @fetchStatsRescheduleTimeout = null
          @fetchStats()
        , FETCH_STATS_NEXT
        return

      return if @fetchStatsPending
      @fetchStatsRetryTimeout = window.setTimeout =>
        @fetchStatsRetryTimeout = null
        @fetchStats(retryCount-1)
      , FETCH_STATS_RETRY
    )

  getStats: ->
    return unless @get('isRunning')
    Em.RSVP.Promise.resolve(
      Em.$.ajax(
        type: 'GET',
        url: "/api/v0/hosts/#{@get 'slug'}/stats",
        headers: { Authorization: "Bearer #{App.get 'token'}" },
        data:
          period: '1s'
      )).then(
      (data) =>
        @set 'connectivityErrorCount', 0
        if stats = data.stats
          @set 'stats', App.HostStats.create(stats)
        data
      , (jqxhr) =>
        @incrementProperty 'connectivityErrorCount'
        throw jqxhr
    )

klass.reopenClass
  url: '/hosts'
  primaryKey: 'slug'
  rootKey: 'host'
  collectionKey: 'hosts'

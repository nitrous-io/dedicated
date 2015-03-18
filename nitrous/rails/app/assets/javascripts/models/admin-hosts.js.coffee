#= require ./model
attr = SM.attr

App.AdminHosts = klass = App.Model.extend
  _dbHosts: attr('array', as: 'db_hosts')
  _liveHosts: attr('array', as: 'live_hosts')

  dbHosts: (->
    liveHosts = @get('liveHosts')
    @get('_dbHosts').map (attrs) ->
      primaryKey = {}
      primaryKey[App.Host.primaryKey] = attrs[App.Host.primaryKey]
      host = App.Host.newRecord(primaryKey)
      host.load(attrs)
      host.set 'preprovisioned', attrs.preprovisioned
      liveHost = liveHosts.findBy('id', host.get('providerIdentifier'))
      if liveHost
        host.set('liveHost', liveHost)
      host
  ).property '_dbHosts', 'liveHosts'

  liveHosts: (->
    @get('_liveHosts').map (attrs) ->
      host = App.LiveHost.create()
      host.load(attrs)
      host
  ).property '_liveHosts'

  # Hosts that are in Hostbuilder but not in DB.
  _unaccounted: attr('object', as: 'unaccounted')
  unaccounted: (->
    dbHosts = @get('dbHosts')
    @get('liveHosts').filter (liveHost) ->
      !dbHosts.isAny('providerIdentifier', liveHost.get('id'))
  ).property 'dbHosts.@each', 'liveHosts.@each'

  # Hosts that are not running (according to DB or Hostbuilder).
  _nonRunning: attr('object', as: 'non_running')
  nonRunning: (->
    @get('dbHosts').filter (dbHost) ->
      return true unless dbHost.get('isRunning')
      liveHost = dbHost.get('liveHost')
      return false unless liveHost
      !liveHost.get('isRunning')
  ).property 'dbHosts.@each', 'liveHosts.@each'

  # Hosts that are in DB but not in Hostbuilder.
  _dangling: attr('object', as: 'dangling')
  dangling: (->
    @get('dbHosts').filter (dbHost) ->
      !dbHost.get('liveHost')
  ).property 'dbHosts.@each', 'liveHosts.@each'

  preprovisioned: (->
    @get('ok').filter (okHost) ->
      okHost.get('preprovisioned')
  ).property 'ok'

  assigned: (->
    @get('ok').filter (okHost) ->
      !okHost.get('preprovisioned')
  ).property 'ok'

  # Hosts that are fine.
  _ok: attr('object', as: 'ok')
  ok: (->
    @get('dbHosts').filter (dbHost) ->
      return false unless dbHost.get('isRunning')
      liveHost = dbHost.get('liveHost')
      return false unless liveHost
      liveHost.get('isRunning')
  ).property 'dbHosts.@each', 'liveHosts.@each'

klass.reopenClass
  fetch: (provider) ->
    new Ember.RSVP.Promise (resolve, reject) =>
      @adapter.ajax('GET', '/admin/hosts',
        provider: provider
      ).done((data) =>
        hosts = App.AdminHosts.create()
        hosts.load data
        resolve(hosts)
      ).fail (jqxhr) ->
        reject(jqxhr)

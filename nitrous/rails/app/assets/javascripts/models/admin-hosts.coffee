#= require ./model
attr = SM.attr

loadHost = (attrs) ->
  primaryKey = {}
  primaryKey[App.Host.primaryKey] = attrs[App.Host.primaryKey]
  host = App.Host.newRecord(primaryKey)
  host.load(attrs)
  host

App.AdminHosts = klass = App.Model.extend
  _unaccounted: attr('object', as: 'unaccounted')
  unaccounted: (->
    @get('_unaccounted').map (attrs) ->
      host = App.FreighterHost.create()
      host.load(attrs)
      host
  ).property '_unaccounted'

  _nonRunning: attr('object', as: 'non_running')
  nonRunning: (->
    @get('_nonRunning').map (attrs) ->
      loadHost(attrs)
  ).property '_nonRunning'

  _dangling: attr('object', as: 'dangling')
  dangling: (->
    @get('_dangling').map (attrs) ->
      loadHost(attrs)
  ).property '_dangling'

  _ok: attr('object', as: 'ok')
  ok: (->
    @get('_ok').map (attrs) ->
      loadHost(attrs)
  ).property '_ok'

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

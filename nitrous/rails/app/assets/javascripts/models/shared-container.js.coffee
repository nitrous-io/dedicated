#= require ./model
attr = SM.attr

App.SharedContainer = klass = App.Container.extend
  userUsername: attr('string', as: 'user_username', serialize: false)
  userName: attr('string', as: 'user_name', serialize: false)
  userGravatarHash: attr('string', as: 'user_gravatar_hash', serialize: false)

  _host: attr('host', as: 'host', serialize: false)

  shared: true

  loadConfig: ->
    @ajax('GET', 'config').then (json) =>
      @setConfig(json.config)

  host: (->
    h = App.Host.create()
    h.load @get('_host')
    h
  ).property '_host'

klass.reopenClass
  primaryKey: 'slug'
  url: '/containers'
  multipleUrl: '/shared_containers'
  rootKey: 'container'
  collectionKey: 'containers'
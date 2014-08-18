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
  ipAddress: attr(key: 'ip_address', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)
  terminatedAt: attr('date', as: 'terminated_at', serialize: false)

  displayName: (->
    @get('name') || @get('slug')
  ).property 'name', 'slug'

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

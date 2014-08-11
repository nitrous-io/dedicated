#= require ./model
attr = SM.attr

App.Container = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', key: 'user_id', serialize: false)
  hostId: attr('number', key: 'host_id', serialize: false)
  image: attr()
  slug: attr()
  name: attr()
  state: attr(serialize: false)
  ipAddress: attr(key: 'ip_address', serialize: false)
  createdAt: attr('date', key: 'created_at', serialize: false)
  updatedAt: attr('date', key: 'updated_at', serialize: false)
  terminatedAt: attr('date', key: 'terminated_at', serialize: false)

  displayName: (->
    @get('name') || @get('slug')
  ).property 'slug', 'name'

  selected: (->
    App.get('selectedContainer') == this
  ).property 'App.selectedContainer'

klass.reopenClass
  primaryKey: 'slug'
  url: '/containers'
  rootKey: 'container'
  collectionKey: 'containers'

#= require ./model
attr = SM.attr

App.Host = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', key: 'user_id', serialize: false)
  slug: attr()
  name: attr()
  state: attr(serialize: false)
  ipAddress: attr(key: 'ip_address', serialize: false)
  freighterdPort: attr('number', key: 'freighterd_port', serialize: false)
  createdAt: attr('date', key: 'created_at', serialize: false)
  updatedAt: attr('date', key: 'updated_at', serialize: false)
  terminatedAt: attr('date', key: 'terminated_at', serialize: false)

  displayName: (->
    @get('name') || @get('slug')
  ).property 'slug', 'name'

  containers: (->
    if slug = @get('slug')
      App.Container.fetch(host: slug).value
  ).property('slug')

  selected: (->
    App.get('selectedHost') == this
  ).property 'App.selectedHost'

klass.reopenClass
  url: '/hosts'
  primaryKey: 'slug'
  rootKey: 'host'
  collectionKey: 'hosts'

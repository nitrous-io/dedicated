#= require ./model
attr = SM.attr

App.Host = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  slug: attr()
  name: attr()
  state: attr(serialize: false)
  ipAddress: attr(as: 'ip_address', serialize: false)
  freighterdPort: attr('number', as: 'freighterd_port', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)
  terminatedAt: attr('date', as: 'terminated_at', serialize: false)

  displayName: (->
    @get('name') || @get('slug')
  ).property 'name', 'slug'

  containers: (->
    if id = @get('id')
      App.get('containers')?.filterBy('hostId', id)
  ).property('id', 'App.containers.@each')

  selected: (->
    App.get('selectedHost') == this
  ).property 'App.selectedHost'

klass.reopenClass
  url: '/hosts'
  primaryKey: 'slug'
  rootKey: 'host'
  collectionKey: 'hosts'

#= require ./model
attr = SM.attr

App.Registry = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  url: attr()
  username: attr()
  email: attr()
  password: attr()
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)

  inputId: (->
    if id = @get 'id'
      "registry-#{id}"
  ).property 'id'

klass.reopenClass
  url: '/hosts/:hostId/registries'
  primaryKey: 'url'
  collectionKey: 'registries'

#= require ./model
attr = SM.attr

App.Volume = klass = App.Model.extend
  id: attr('number', serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  name: attr()
  slug: attr()
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)

  displayName: (->
    @get('name') || @get('slug')
  ).property 'name', 'slug'

  inputId: (->
    if id = @get 'id'
      "volume-#{id}"
  ).property 'id'

  defaultMountpoint: (->
    if slug = @get 'slug'
      "/volumes/#{slug}"
  ).property 'slug'

klass.reopenClass
  url: '/hosts/:hostId/volumes'
  primaryKey: 'slug'
  collectionKey: 'volumes'

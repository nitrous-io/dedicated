#= require ./model
attr = SM.attr

App.Image = klass = App.Model.extend
  id: attr(serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  name: attr()
  slug: attr()
  tag: attr()
  actualSize: attr('number', as: 'actual_size', serialize: false)
  virtualSize: attr('number', as: 'virtual_size', serialize: false)
  builtAt: attr('date', as: 'created_at', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'created_at', serialize: false)

  slugAndTag: (->
    slug = @get 'slug'
    tag = @get 'tag'
    "#{slug}:#{tag}"
  ).property 'slug', 'tag'

klass.reopenClass
  url: '/hosts/:hostId/images'
  primaryKey: 'slug'
  rootKey: 'image'
  collectionKey: 'images'

  inputPattern: '[a-z0-9\\._\\-]{3,30}'
  inputTitle: 'May only contain alphanumeric characters, underscores or dashes. (3-30 characters)'

#= require ./model
attr = SM.attr

App.Image = klass = App.Model.extend
  id: attr(serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  name: attr()
  tag: attr()
  actualSize: attr('number', as: 'actual_size')
  virtualSize: attr('number', as: 'virtual_size')
  createdAt: attr('date', as: 'created_at', serialize: false)

  nameAndTag: (->
    name = @get 'name'
    tag = @get 'tag'
    "#{name}:#{tag}"
  ).property 'name', 'tag'

klass.reopenClass
  url: '/hosts/:hostId/images'
  primaryKey: 'name'
  rootKey: 'image'
  collectionKey: 'images'

  inputPattern: '[a-z0-9\._\-]{3,30}'
  inputTitle: 'May only contain alphanumeric characters, underscores or dashes. (3-30 characters)'

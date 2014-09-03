#= require ./model
attr = SM.attr

App.BaseImage = klass = App.Model.extend
  id: attr(serialize: false)
  name: attr()
  repo: attr()
  description: attr()

klass.reopenClass
  url: '/base_images'
  primaryKey: 'id'
  collectionKey: 'base_images'

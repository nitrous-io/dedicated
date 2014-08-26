#= require ./model
attr = SM.attr

App.Region = klass = App.Model.extend
  id: attr('number', serialize: false)
  slug: attr()
  name: attr()

  inputId: (->
    if slug = @get 'slug'
      "region-#{slug}"
  ).property 'slug'

klass.reopenClass
  url: '/regions'
  primarykey: 'id'
  collectionKey: 'regions'

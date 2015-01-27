#= require ./model
attr = SM.attr

iconBucketUrl = "#{location.protocol}//nitrous-image-icons.s3.amazonaws.com"

App.Region = klass = App.Model.extend
  id: attr('number', serialize: false)
  slug: attr()
  name: attr()

  inputId: (->
    if slug = @get 'slug'
      "region-#{slug}"
  ).property 'slug'

  iconUrl: (->
    return unless slug = @get('slug')
    "#{iconBucketUrl}/regions/#{slug}.png"
  ).property 'slug'

klass.reopenClass
  url: '/regions'
  primaryKey: 'id'
  collectionKey: 'regions'

  defaultIconUrl: "#{iconBucketUrl}/regions/default.png"

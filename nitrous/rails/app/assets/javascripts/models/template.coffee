#= require ./model
attr = SM.attr

iconBucketUrl = "#{location.protocol}//nitrous-image-icons.s3.amazonaws.com"

App.Template = klass = App.Model.extend
  id: attr(serialize: false)
  slug: attr()
  name: attr()
  description: attr()
  type: attr()
  image: attr()
  privileged: attr('boolean', default: false)

  iconUrl: (->
    return unless slug = @get('slug')
    "#{iconBucketUrl}/#{slug}.png"
  ).property 'slug'

klass.reopenClass
  url: '/templates'
  primaryKey: 'id'
  collectionKey: 'templates'

  defaultIconUrl: "#{iconBucketUrl}/nitrous.png"

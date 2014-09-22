#= require ./model
attr = SM.attr

App.Image = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  freighterId: attr(as: 'freighter_id', serialize: false)
  name: attr()
  repo: attr()
  slug: attr()
  tag: attr()
  actualSize: attr('number', as: 'actual_size', serialize: false)
  virtualSize: attr('number', as: 'virtual_size', serialize: false)
  builtAt: attr('date', as: 'created_at', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'created_at', serialize: false)
  progress: attr('json', serialize: false)

  updating: false

  slugAndTag: (->
    slug = @get 'slug'
    tag = @get 'tag'
    "#{slug}:#{tag}"
  ).property 'slug', 'tag'

  displayName: (->
    @get('name') || @get('slugAndTag')
  ).property 'name', 'slugAndTag'

  inputId: (->
    if id = @get 'id'
      "image-#{id}"
  ).property 'id'

klass.reopenClass
  url: '/hosts/:hostId/images'
  primaryKey: 'id'
  rootKey: 'image'
  collectionKey: 'images'

  inputPattern: '[a-z0-9\\._\\-]{3,30}'
  inputTitle: 'May only contain alphanumeric characters, underscores or dashes. (3-30 characters)'

  pull: (host, repository, update=true) ->
    promise = new Ember.RSVP.Promise (resolve, reject) =>
      @adapter.ajax('POST', "/hosts/#{host.get 'slug'}/images/pull",
        repository: repository
        update: update
        async: true
      ).done((data) =>
        attrs = data[@rootKey]
        primaryKey = {}
        primaryKey[@primaryKey] = attrs[@primaryKey]
        image = App.Image.newRecord(primaryKey)
        image.load(attrs)
        resolve(image)
      ).fail (jqxhr) ->
        reject(jqxhr)

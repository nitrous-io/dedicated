App.NewModel = klass = Em.Object.extend
  recordUrl: (->
    throw new Error("recordUrl property not implemented")
  ).property()

  serialize: ->
    throw new Error("serialize method not implemented")

  load: (data={}, deserialized=false) ->
    data = @constructor.deserialize(data) unless deserialized
    @setPropertiesIfDefined(data)
    @cache()
    this

  save: ->
    id = @get 'id'
    unless id
      # create
      new Ember.RSVP.Promise (resolve, reject) =>
        Em.$.ajax(
          type: 'POST',
          url: Em.get(@constructor, 'collectionUrl')
          dataType: 'json'
          headers: { Authorization: "Bearer #{App.get 'token'}" }
          data: stripUndefined(@serialize())
        ).done((data, status, jqxhr) =>
          if d = data[Em.get(@constructor, 'recordRoot')]
            @load(d)
            resolve(this)
          else
            reject(jqxhr)
        ).fail (jqxhr) ->
          reject(jqxhr)

  cache: ->
    primaryKey = Em.get(@constructor, 'primaryKey')
    return unless key = @get(primaryKey)
    (@constructor.cache ?= {})[key] = this

  setIfDefined: (key, val) ->
    @set(key, val) if val != undefined

  setPropertiesIfDefined: (data) ->
    @setProperties(stripUndefined(data))

klass.reopenClass
  primaryKey: 'id'

  collectionUrl: (->
    throw new Error("collectionUrl property not implemented")
  ).property()

  collectionRoot: (->
    throw new Error("collectionRoot property not implemented")
  ).property()

  recordRoot: (->
    throw new Error("recordRoot property not implemented")
  ).property()

  fromCache: (key) ->
    (@cache ?= {})[key]

  fetch: (existingCollection=null, query={}) ->
    new Ember.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'GET'
        url: Em.get(this, 'collectionUrl')
        data: query
        dataType: 'json'
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).done((data, status, jqxhr) =>
        if ds = data[Em.get(this, 'collectionRoot')]
          records = ds.map (d) =>
            deserialized = @deserialize(d)
            key = deserialized[Em.get(this, 'primaryKey')]
            record = @fromCache(key) ? @create()
            record.load(deserialized, true)
          # if existingCollection is given, it is merged with records fetched
          if existingCollection
            return resolve(@diffMerge(existingCollection, records))
          resolve(records)
        else
          reject(jqxhr)
      ).fail (jqxhr) ->
        reject(jqxhr)

  diffMerge: (existing, newArray) ->
    removed = []
    added = []

    for record in existing
      removed.push(record) if !newArray.contains(record)
    for record in newArray
      added.push(record) if !existing.contains(record)

    existing.removeObjects(removed)
    existing.pushObjects(removed)
    existing

stripUndefined = (data) ->
  d = {}
  for own k, v of data
    d[k] = v unless v == undefined
  d

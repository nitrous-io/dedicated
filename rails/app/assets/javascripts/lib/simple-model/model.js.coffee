#= require ./sm

SM.Model = klass = Ember.Object.extend
  _data: null

  serialize: ->
    attrs = Ember.get @constructor, '_attrs'
    serialized = {}
    attrs.forEach (key, attr) =>
      if attr.options.serialize
        val = Ember.get this, key
        if val
          if val.toJSON && typeof val.toJSON == 'function'
            val = val.toJSON()
          serialized[@constructor._getKey(key)] = val
    serialized

  load: (data) ->
    @setProperties(@constructor.deserialize(data))
    @_saveToCache()

  getPrimaryKey: ->
    @get(Em.get @constructor, 'primaryKey')

  _saveToCache: ->
    if primaryKey = @getPrimaryKey()
      unless @constructor._findInCache(primaryKey)
        Ember.get(@constructor, '_cache').set primaryKey, this

  save: ->
    adapter = Ember.get(@constructor, 'adapter')
    if primaryKey = @getPrimaryKey()
      # TODO: update record
    else
      adapter.createRecord(this)

klass.reopenClass
  primaryKey: 'id'
  adapter: null

  _cache: Ember.computed ->
    Ember.Map.create()

  _attrs: Ember.computed ->
    map = Ember.Map.create()
    @eachComputedProperty (name, meta) ->
      if meta.isAttribute
        map.set name, meta
    map

  _getAttr: (propertyName) ->
    Ember.get(this, '_attrs').get(propertyName)

  _getKey: (propertyName) ->
    @_getAttr(propertyName).options.as || propertyName

  _findInCache: (primaryKey) ->
    cache = Ember.get this, '_cache'
    cache.get(primaryKey)

  _removeFromCache: (primaryKey) ->
    cache = Ember.get this, '_cache'
    cache.remove(primaryKey)

  deserialize: (data) ->
    d = {}
    Ember.get(this, '_attrs').forEach (key, attr) =>
      k = @_getKey(key)
      if data.hasOwnProperty(k)
        if (val = data[k])?
          val = switch attr.type
            when 'string'
              String(val)
            when 'date'
              new Date(val)
            when 'number'
              Number(val)
            else
              val
        d[key] = val
    d

  fetch: ->
    adapter = Ember.get this, 'adapter'
    adapter.fetch.apply(adapter, [this].concat(Ember.$.makeArray(arguments)))

SM.attr = (type, options) ->
  if typeof type != "string" && !options
    options = type
    type = 'string'
  options ||= {}
  options.serialize ?= true

  meta =
    isAttribute: true
    type: type
    options: options

  Ember.computed('_data', (key, val) ->
    data = Ember.get(this, '_data') || Ember.set(this, '_data', {})
    if arguments.length > 1
      data[key] = val
    data[key]
  ).meta(meta)

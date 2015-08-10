#= require ./sm

SM.Model = klass = Ember.Object.extend
  _data: null
  loaded: false

  init: ->
    @_super.apply this, arguments
    attrs = Ember.get @constructor, '_attrs'
    attrs.forEach (key, attr) =>
      defaultVal = attr.options.default
      if defaultVal != undefined
        @set key, defaultVal

  serialize: ->
    attrs = Ember.get @constructor, '_attrs'
    serialized = {}
    attrs.forEach (key, attr) =>
      if attr.options.serialize
        val = Ember.get this, key
        if val != null && val != undefined
          if attr.type == 'json'
            val = JSON.stringify(val)
          else if val.toJSON && typeof val.toJSON == 'function'
            val = val.toJSON()
          serialized[@constructor._getKey(key)] = val
    serialized

  load: (data, options={}) ->
    unless options.deserialize? && options.deserialize == false
      data = @constructor.deserialize(data)
    @setProperties(data)
    @set 'loaded', true
    @_saveToCache()
    this

  getPrimaryKey: ->
    @get(Ember.get @constructor, 'primaryKey')

  _saveToCache: ->
    return if Ember.get(@constructor, 'noCache')
    cache = Ember.get(@constructor, '_cache')
    if Ember.get(@constructor, 'singular')
      cache.set 1, this
      return
    if primaryKey = @getPrimaryKey()
      cache.set primaryKey, this

  save: (params, options) ->
    adapter = Ember.get(@constructor, 'adapter')
    if @getPrimaryKey() && @get('loaded')
      adapter.updateRecord(this, params, options)
    else
      adapter.createRecord(this, params, options)

  forceUpdate: (params, options) ->
    adapter = Ember.get(@constructor, 'adapter')
    adapter.updateRecord(this, params, options)

klass.reopenClass
  primaryKey: 'id'
  singular: false
  adapter: null
  noCache: false

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
    return if Ember.get(this, 'noCache')
    if Ember.get this, 'singular'
      primaryKey = 1
    cache = Ember.get this, '_cache'
    cache.get(primaryKey)

  _removeFromCache: (primaryKey) ->
    cache = Ember.get this, '_cache'
    cache.remove(primaryKey)

  newRecord: (props={}) ->
    r = @_findInCache(props[Ember.get(this, 'primaryKey')])
    if r
      r.setProperties(props)
    r ||= @create(props)

  deserialize: (data) ->
    d = {}
    return d unless data
    Ember.get(this, '_attrs').forEach (key, attr) =>
      k = @_getKey(key)
      if data.hasOwnProperty(k) || attr.options?.overwrite
        if (val = data[k])?
          val = switch attr.type
            when 'string'
              String(val)
            when 'date'
              new Date(val)
            when 'number'
              Number(val)
            when 'json'
              JSON.parse(val || null)
            else
              val
        d[key] = val
    d

  fetch: (primaryKey, params) ->
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

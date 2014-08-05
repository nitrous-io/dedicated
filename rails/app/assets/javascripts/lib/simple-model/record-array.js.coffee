#= require ./sm

SM.RecordArray = Ember.ArrayProxy.extend Ember.Evented,
  model: null

  load: (data) ->
    model = @get 'model'
    @set 'content', Em.A() unless @get('content')?
    @pushObjects data.map (d) ->
      props = model.deserialize(d)
      record = null
      if model.primaryKey && (key = props[model.primaryKey])?
        record = model._findInCache(key)
      record ||= model.create()
      record.setProperties(props)
      record._saveToCache()
      record

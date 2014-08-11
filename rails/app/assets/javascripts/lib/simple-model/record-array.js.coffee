#= require ./sm

SM.RecordArray = Ember.ArrayProxy.extend Ember.Evented,
  model: null
  loaded: false

  load: (data) ->
    model = @get 'model'
    @set 'content', Ember.A() unless @get('content')?
    @pushObjects data.map (d) ->
      props = model.deserialize(d)
      pkey = Ember.get(model, 'primaryKey')
      keyHash = {}
      keyHash[pkey] = props[pkey]
      record = model.newRecord(keyHash)
      record.load(props, deserialize: false)
      record
    @set 'loaded', true

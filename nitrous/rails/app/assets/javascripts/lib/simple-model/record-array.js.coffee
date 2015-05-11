#= require ./sm

SM.RecordArray = Ember.ArrayProxy.extend Ember.Evented,
  model: null
  loaded: false

  errors: []

  load: (data) ->
    model = @get 'model'
    unless existing = @get('content')
      @set 'content', existing = Em.A()

    newRecords = Ember.A data.map (d) ->
      props = model.deserialize(d)
      pkey = Ember.get(model, 'primaryKey')
      keyHash = {}
      keyHash[pkey] = props[pkey]
      record = model.newRecord(keyHash)
      record.load(props, deserialize: false)
      record

    removed = Em.A()
    added = Em.A()

    for record in existing
      removed.push(record) if !newRecords.contains(record)
    for record in newRecords
      added.push(record) if !existing.contains(record)

    existing.removeObjects(removed)
    existing.pushObjects(added)

    @set 'loaded', true
    @trigger 'loaded'

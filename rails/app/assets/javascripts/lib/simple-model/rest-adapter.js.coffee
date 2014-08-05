#= require ./model

SM.RESTAdapter = Ember.Object.extend
  headers: {}

  namespace: ''

  ajax: (method, url, data=null, headers={}) ->
    headers = Ember.merge(Ember.merge({}, Ember.get(this, 'headers')), headers)

    if namespace = Ember.get(this, 'namespace')
      url = namespace + url

    Ember.$.ajax
      type: method
      url: url
      data: data
      headers: headers

  fetch: (model, primaryKey) ->
    singular = Ember.get(model, 'singular')
    url = Ember.get(model, 'url')

    if singular
      single = true
    else if !singular && primaryKey?
      single = true
      url = "#{url}/#{primaryKey}"
    else
      single = false

    r = if single
      model.create()
    else
      SM.RecordArray.create(model: model)

    new Ember.RSVP.Promise (resolve, reject) =>
      @ajax('GET', url).done((data) ->
        if single && model.rootKey
          data = data[model.rootKey] || {}
        else if !single && model.collectionKey
          data = data[model.collectionKey] || {}
        r.load(data)
        resolve(r)
      ).fail (jqxhr) ->
        reject(jqxhr)

  createRecord: (record) ->
    model = record.constructor
    url = Ember.get(model, "createURL") || Ember.get(model, "url")
    new Ember.RSVP.Promise (resolve, reject) =>
      @ajax('POST', url, record.serialize()).done((data, status, jqxhr) ->
        if model.rootKey
          data = data[model.rootKey] || {}
        record.load(data)
        resolve(record)
      ).fail (jqxhr) ->
        reject(jqxhr)

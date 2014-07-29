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
      model.create(isLoading: true)
    else
      SM.RecordArray.create(model: model, isLoading: true)

    @ajax('GET', url).done((data, status, jqxhr) ->
      if single && model.rootKey
        data = data[model.rootKey] || {}
      else if !single && model.collectionKey
        data = data[model.collectionKey] || {}
      r.load(data)
      r.setProperties
        isLoading: false
        isLoaded: true
      r.trigger 'didLoad', r, jqxhr
    ).fail (jqxhr) ->
      r.setProperties
        isLoading: false
        isLoaded: false
      r.trigger 'didFailToLoad', r, jqxhr
    r

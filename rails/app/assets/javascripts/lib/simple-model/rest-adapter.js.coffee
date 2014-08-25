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

  fetch: (model, primaryKey, params) ->
    singular = Ember.get(model, 'singular')
    url = Ember.get(model, 'url')

    if typeof primaryKey != 'number' && typeof primaryKey != 'string' && typeof primaryKey == 'object'
      params = primaryKey
      primaryKey = undefined

    if singular
      single = true
    else if !singular && primaryKey?
      single = true
      url = "#{url}/#{encodeURIComponent(primaryKey)}"
    else
      single = false

    if params
      # handle dynamic segments
      urlComps = url.split('/')
      for k, v of params
        if params.hasOwnProperty(k)
          i = urlComps.indexOf(':'+k)
          if i != -1
            urlComps[i] = v
            delete params[k]
      url = urlComps.join('/')

      # add the rest of the params as query string
      query = Ember.$.map(params, (v, k) ->
        "#{k}=#{encodeURIComponent(v)}"
      ).join("&")
      url = "#{url}?#{query}" if query

    r = if single
      props = {}
      if primaryKey
        props[Ember.get(model, 'primaryKey')] = primaryKey
      model.newRecord(props)
    else
      SM.RecordArray.create(model: model)

    promise = new Ember.RSVP.Promise (resolve, reject) =>
      @ajax('GET', url).done((data, status, jqxhr) ->
        if single && model.rootKey
          data = data[model.rootKey] || {}
        else if !single && model.collectionKey
          data = data[model.collectionKey] || {}
        if single
          r.load(data)
          resolve(r)
          return
        if Em.isArray(data)
          r.load(data)
          resolve(r)
          return
        reject(jqxhr)
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise.value = r
    promise

  createRecord: (record) ->
    model = record.constructor
    url = Ember.get(model, "createURL") || Ember.get(model, "url")

    url = @_fillDynamicSegments(url, record)

    promise = new Ember.RSVP.Promise (resolve, reject) =>
      @ajax('POST', url, record.serialize()).done((data, status, jqxhr) ->
        if model.rootKey
          data = data[model.rootKey] || {}
        record.load(data)
        resolve(record)
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise.value = record
    promise

  _fillDynamicSegments: (url, data) ->
    urlComps = url.split('/')
    for comp, i in urlComps
      if comp.charAt(0) == ':' && comp.length > 1
        if v = Ember.get(data, comp.slice(1))
          urlComps[i] = v
    urlComps.join('/')

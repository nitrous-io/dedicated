#= require ./model

SM.RESTAdapter = Ember.Object.extend
  headers: {}

  namespace: ''

  ajax: (method, url, data=null, headers={}, options={}) ->
    headers = Ember.merge(Ember.merge({}, Ember.get(this, 'headers')), headers)

    if namespace = Ember.get(this, 'namespace')
      url = namespace + url

    ajaxOptions =
      type: method
      url: url
      data: data
      headers: headers
    Ember.merge(ajaxOptions, options)
    Ember.$.ajax(ajaxOptions)

  fetch: (model, primaryKey, params) ->
    singular = Ember.get(model, 'singular')
    url = Ember.get(model, 'url')

    if typeof primaryKey != 'number' && typeof primaryKey != 'string' && typeof primaryKey == 'object'
      params = primaryKey
      primaryKey = undefined

    if singular
      fetchOne = true
    else if !singular && primaryKey?
      fetchOne = true
      url = "#{url}/#{encodeURIComponent(primaryKey)}"
    else
      if mulUrl = Ember.get(model, 'multipleUrl')
        url = mulUrl
      fetchOne = false

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

    if fetchOne
      attrs = {}
      if primaryKey
        attrs[Ember.get(model, 'primaryKey')] = primaryKey
      record = model.newRecord(attrs)
      @_fetchOne(model, record, url)
    else
      recordArray = SM.RecordArray.create(model: model)
      @_fetchMultiple(model, recordArray, url)

  _fetchOne: (model, record, url) ->
    args = $.makeArray(arguments)
    record.set 'reload', =>
      @_fetchOne.apply(this, args)
    promise = new Ember.RSVP.Promise (resolve, reject) =>
      @ajax('GET', url).done((data, status, jqxhr) ->
        if rootKey = Ember.get(model, 'rootKey')
          data = data[rootKey] || {}
        record.load(data)
        resolve(record)
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise.value = record
    promise

  _fetchMultiple: (model, recordArray, url)->
    args = $.makeArray(arguments)
    recordArray.set 'reload', =>
      @_fetchMultiple.apply(this, args)
    promise = new Ember.RSVP.Promise (resolve, reject) =>
      @ajax('GET', url).done((data, status, jqxhr) ->
        if collectionKey = Ember.get(model, 'collectionKey')
          data = data[collectionKey] || {}
        if Ember.isArray(data)
          recordArray.load(data)
          resolve(recordArray)
          return
        reject(jqxhr)
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise.value = recordArray
    promise

  updateRecord: (record, params, options={}) ->
    model = record.constructor
    url = Ember.get(model, "updateURL") || Ember.get(model, "url")
    @_performResourceAction url, 'PUT', record, params, options

  createRecord: (record, params, options={}) ->
    model = record.constructor
    url = Ember.get(model, "createURL") || Ember.get(model, "url")
    @_performResourceAction url, 'POST', record, params, options

  _performResourceAction: (url, method, record, params, options={}) ->
    model = record.constructor
    url = @_fillDynamicSegments(url, record)

    promise = new Ember.RSVP.Promise (resolve, reject) =>
      serialized = record.serialize()
      Ember.merge(serialized, params)

      rootKey = Ember.get(model, 'rootKey')
      if options.json == true && rootKey
        json = {}
        json[rootKey] = serialized
        serialized = JSON.stringify(json)
        options.contentType = 'application/json; charset=utf-8'
        delete options.json

      @ajax(method, url, serialized, null, options).done((data, status, jqxhr) ->
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

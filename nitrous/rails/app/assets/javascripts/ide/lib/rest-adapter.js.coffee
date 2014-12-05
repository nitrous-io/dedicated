App.RESTAdapter = Em.RESTAdapter.extend
  makeRequest: (record, action, method='GET', params)  ->
    recordClass = record.constructor
    urlRoot = recordClass.url
    id = record.get(recordClass.primaryKey)
    url = @buildURL(recordClass, id)
    url = url.replace /\.json$/, "/#{action}.json" if action
    @ajax url, params, method

#= require ../lib/rest-adapter
#= require ../lib/simple-model/model

App.Model = klass = SM.Model.extend
  ajax: (method, action, data=null, headers={}, options={}) ->
    baseUrl = Ember.get(@constructor, 'url')
    urlComponents = [baseUrl]
    unless Ember.get(@constructor, 'singular')
      urlComponents.push(encodeURIComponent(@getPrimaryKey()))
    urlComponents.push(encodeURIComponent(action)) if action
    adapter = @constructor.adapter
    url = adapter._fillDynamicSegments(urlComponents.join('/'), this)
    Em.RSVP.Promise.cast(adapter.ajax(method, url, data, headers, options))

klass.reopenClass
  adapter: App.RESTAdapter.create()

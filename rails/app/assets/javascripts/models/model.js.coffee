#= require ../lib/rest-adapter
#= require ../lib/simple-model/model

App.Model = klass = SM.Model.extend
  ajax: (method, action, data=null) ->
    urlComponents = [Ember.get(@constructor, 'url')]
    unless Ember.get(@constructor, 'singular')
      urlComponents.push(@getPrimaryKey())
    urlComponents.push(action) if action?
    Em.RSVP.Promise.cast(@constructor.adapter.ajax(method, urlComponents.join('/'), data))

klass.reopenClass
  adapter: App.RESTAdapter.create()

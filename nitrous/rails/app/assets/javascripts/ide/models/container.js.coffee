#= require ./base-model

App.Container = klass = Em.Object.extend
  id: null
  userId: null
  hostId: null
  slug: null
  name: null
  state: null

  # FIXME this should be serialized in model
  freighterdUrl: (->
    "test-4771.n2odev.com:49159"
  ).property 'slug'

  # FIXME load config and get this from env
  loginUsername: 'action'

  requestToken: (permissions) ->
    promise = new Ember.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'POST',
        url: "/api/v0/containers/#{@get 'slug'}/token",
        headers: { Authorization: "Bearer #{App.get 'token'}" }
        data: {
          permission: permissions
        }
      ).done((data, status, jqxhr) ->
        resolve(data)
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise

klass.reopenClass
  fetch: (slug) ->
    promise = new Ember.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'GET',
        url: "/api/v0/containers/#{slug}",
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).done((data, status, jqxhr) ->
        resolve(App.Container.create(data.container))
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise

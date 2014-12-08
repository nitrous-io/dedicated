#= require ./base-model

App.Container = klass = Em.Object.extend
  id: null
  userId: null
  hostId: null
  image: null
  slug: null
  name: null
  state: null
  createdAt: null
  updatedAt: null
  startedAt: null
  finishedAt: null
  terminatedAt: null

  host: (->
    if hostId = @get 'hostId'
      App.get('hosts')?.findBy('id', hostId)
  ).property 'hostId', 'App.hosts.@each'

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

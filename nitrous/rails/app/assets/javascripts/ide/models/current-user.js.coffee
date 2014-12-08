#= require ./base-model

App.CurrentUser = klass = App.BaseModel.extend
  username: Em.attr()
  email: Em.attr()
  full_name: Em.attr()
  created_at: Em.attr()
  admin: Em.attr()
  confirmed_at: Em.attr()

  init: ->
    @_super.apply this, arguments

klass.reopenClass
  adapter: null # do REST manually for this class
  rootKey: 'user'

  fetch: ->
    promise = new Ember.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'GET',
        url: '/api/v0/user',
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).done((data, status, jqxhr) ->
        resolve(App.CurrentUser.create(data.user))
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise

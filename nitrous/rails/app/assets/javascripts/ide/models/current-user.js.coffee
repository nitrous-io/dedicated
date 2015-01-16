App.CurrentUser = klass = Em.Object.extend
  username: null
  email: null
  full_name: null
  created_at: null
  admin: null
  confirmed_at: null

  init: ->
    @_super.apply this, arguments

  gravatar: (->
    "https://secure.gravatar.com/avatar/#{md5(@get 'email')}?default=retro&secure=true&size=48"
  ).property 'email'


klass.reopenClass
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

#= require ./user
attr = SM.attr

App.AdminUser = klass = App.User.extend
  deletedAt: attr('date', as: 'deleted_at', serialize: false)
  bannedAt: attr('date', as: 'banned_at', serialize: false)
  bannedBy: attr('string', as: 'banned_by', serialize: false)

  billingProfile: (->
    b = App.BillingProfile.newRecord()
    b.load @get('_billingProfile')
    b
  ).property '_billingProfile'

  save: ->
    return

  forceUpdate: ->
    return

  banUser: ->
    Em.$.ajax(
      type: 'DELETE'
      url: "/api/v0/admin/users/#{@get("id")}/ban"
      cache: false
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    )

klass.reopenClass
  newRecord: (hsh)->
    klass.create(hsh)

  fetchUsers: (slug)->
    users =  SM.RecordArray.create(model: klass)
    promise = new Ember.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'GET'
        url: "/api/v0/admin/users/#{slug}"
        cache: false
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).then (data)=>
        users.load(data.users)
        resolve(users)
      , (jqxhr) =>
        App.postError(jqxhr.responseJSON, 'Failed to load user.')
        reject()
    promise.value = users
    promise
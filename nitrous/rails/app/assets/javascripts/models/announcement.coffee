#= require ./model
attr = SM.attr

App.Announcement = klass = App.Model.extend
  id: attr('number', serialize: false)
  hostId: attr('number', as: 'host_id', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  subject: attr()
  message: attr()
  username: attr()
  slug: attr()
  expiresAt: attr('date', as: 'expires_at', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)

  splitMessage: (->
    if msg = @get 'message'
      return msg.split(/\n/gi).map((ln) ->
        if ln == ""
          return {break: true}
        {text: ln}
      )
    []
  ).property 'message'

klass.reopenClass
  url: '/hosts/:hostId/announcements'
  primaryKey: 'id'
  collectionKey: 'announcements'

#= require ./model
attr = SM.attr

App.UserKey = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  containerId: attr('number', as: 'container_id', serialize: false)
  fingerprint: attr(serialize: false)
  key: attr()
  createdAt: attr('date', as: 'created_at', serialize: false)

  inputId: (->
    if id = @get 'id'
      "key-#{id}"
  ).property 'id'

  name: (->
    id = @get 'id'
    fingerprint = @get 'fingerprint'
    key = @get 'key'
    name = key?.match(/^\s*ssh-(rsa|dss)\s+\S+\s+(\S+)/)?[2] || "Public Key #{id}"
    "#{name} (#{fingerprint})"
  ).property 'id', 'fingerprint', 'key'

klass.reopenClass
  url: '/containers/:containerId/keys'
  primaryKey: 'fingerprint'
  collectionKey: 'keys'

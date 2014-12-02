#= require ./base-model

App.PublicKey = klass = App.BaseModel.extend
  name: Em.attr()
  key: Em.attr()
  md5_fingerprint: Em.attr()
  user_id: Em.attr()

  validations:
    key: (v) -> "can't be blank" unless v && v.length > 0

klass.reopenClass
  rootKey: 'public_key'
  collectionKey: 'public_keys'
  url: '/public_keys'

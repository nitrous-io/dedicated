#= require ./model
attr = SM.attr

App.User = klass = App.Model.extend
  id: attr('number', serialize: false)
  username: attr()
  name: attr()
  email: attr()
  password: attr()
  admin: attr('boolean', default: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)

  advanced: (->
    App.get('userProfile').get 'advanced'
  ).property 'App.userProfile.advanced'

  gravatar: (->
    "https://gravatar.com/avatar/#{md5(@get 'email')}?s=48"
  ).property 'email'

klass.reopenClass
  url: '/user'
  createURL: '/users'
  primaryKey: 'id'
  singular: true
  rootKey: 'user'

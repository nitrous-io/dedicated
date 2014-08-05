#= require ./model
attr = SM.attr

App.User = klass = App.Model.extend
  id: attr('number')
  username: attr()
  name: attr()
  email: attr()
  password: attr()
  createdAt: attr('date', key: 'created_at', serialize: false)
  updatedAt: attr('date', key: 'updated_at', serialize: false)

  gravatar: (->
    "https://gravatar.com/avatar/#{md5(@get 'email')}?s=48"
  ).property 'email'

klass.reopenClass
  url: '/user'
  createURL: '/users'
  primaryKey: 'id'
  singular: true
  rootKey: 'user'

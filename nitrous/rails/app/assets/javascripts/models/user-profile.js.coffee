#= require ./model
attr = SM.attr

App.UserProfile = klass = App.Model.extend
  id: attr('number', serialize: false)
  advanced: attr('boolean', default: false)

klass.reopenClass
  url: '/user/profile'
  primaryKey: 'id'
  singular: true
  rootKey: 'user_profile'

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

  # used for sideloading the records, should not access directly
  # instead use `App.userProfile` and `App.billingProfile`.
  _userProfile: attr('object', as: 'user_profile')
  _billingProfile: attr('object', as: 'billing_profile')

  advancedBinding: 'App.userProfile.advanced'

  gravatar: (->
    "https://gravatar.com/avatar/#{md5(@get 'email')}?s=48"
  ).property 'email'

klass.reopenClass
  url: '/user'
  createURL: '/users'
  updateURL: '/users'
  primaryKey: 'id'
  singular: true
  rootKey: 'user'

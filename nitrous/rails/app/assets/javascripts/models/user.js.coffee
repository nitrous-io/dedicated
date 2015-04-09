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
  unconfirmedEmail: attr(as: 'unconfirmed_email', default: null, overwrite: true)
  promoCode: attr(as: 'promo_code')
  firstTime: attr('boolean', as: 'first_time', default: false)

  # used for sideloading the records, should not access directly
  # instead use `App.userProfile` and `App.billingProfile`.
  _userProfile: attr('object', as: 'user_profile')
  _billingProfile: attr('object', as: 'billing_profile')

  advancedBinding: 'App.userProfile.advanced'

  gravatar: (->
    "https://secure.gravatar.com/avatar/#{md5(@get 'email')}?default=retro&secure=true&size=48"
  ).property 'email'

klass.reopenClass
  url: '/user'
  createURL: '/users'
  updateURL: '/users'
  primaryKey: 'id'
  singular: true
  rootKey: 'user'

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
  phone: attr()
  phoneCountryCode: attr(as: 'phone_country_code')
  phoneAuthed: attr('boolean', as: 'phone_authed', serialize: false)
  maxRunningTime: attr('number', as: 'max_running_time', serialize: false)
  refreshPeriod: attr('number', as: 'refresh_period', serialize: false)

  # used for sideloading the records, should not access directly
  # instead use `App.userProfile` and `App.billingProfile`.
  _userProfile: attr('object', as: 'user_profile')
  _billingProfile: attr('object', as: 'billing_profile')

  advancedBinding: 'App.userProfile.advanced'

  gravatar: (->
    "https://secure.gravatar.com/avatar/#{md5(@get 'email')}?default=retro&secure=true&size=48"
  ).property 'email'

  dismissFirstTime: ->
    @set 'firstTime', false
    @ajax 'POST', 'dismiss_first_time'

  updatePhone: (phone, phoneCountryCode) ->
    @ajax 'POST', 'phone',
      phone: phone
      phone_country_code: phoneCountryCode

  resendPhoneCode: ->
    @ajax 'POST', 'resend_phone_code'

  verifyPhone: (code) ->
    @ajax 'POST', 'verify_phone',
      code: code

klass.reopenClass
  url: '/user'
  createURL: '/users'
  updateURL: '/users'
  primaryKey: 'id'
  singular: true
  rootKey: 'user'

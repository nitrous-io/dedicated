#= require ./model
attr = SM.attr

App.Coupon = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  code: attr()

klass.reopenClass
  url: '/coupons'
  primaryKey: 'id'
  rootKey: 'coupon'
  collectionKey: 'coupons'

#= require ./model
attr = SM.attr

App.AdminCreditType = klass = App.Model.extend
  id: attr('number', serialize: false)
  name: attr(serialize: false)
  description: attr(serialize: false)
  defaultValue: attr('number', as: 'default_value', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)

  displayName: (->
    b = @get 'name'
    if desc = @get 'description'
      b = "#{b} - #{desc}"
    if val = @get 'defaultValue'
      b = "#{b} (#{val}c)"
    b
  ).property 'name', 'description', 'defaultValue'

klass.reopenClass
  url: '/admin/credit_types'
  primaryKey: 'id'
  collectionKey: 'credit_types'
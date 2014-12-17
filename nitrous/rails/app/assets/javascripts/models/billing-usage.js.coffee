#= require ./model
attr = SM.attr

App.BillingUsage = klass = App.Model.extend
  totalCharge: attr('number', as: 'total_charge')
  totalChargeDollars: attr('number', as: 'total_charge_dollars')

  _items: attr('object', as: 'items')

  unpaid: (->
    !@get('successfulCharge')
  ).property 'successfulCharge'

  items: (->
    @get('_items').map (item)->
      inv = App.InvoiceItem.create()
      inv.load item
      inv
  ).property '_items'

klass.reopenClass
  fetchCurrentUsage: ->
    adapter = Ember.get(this, 'adapter');
    adapter.ajax('GET', '/billing/current_usage').then (data) =>
      inst = App.BillingUsage.create()
      inst.load data
      App.set 'billingUsage', inst
      inst

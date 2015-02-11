#= require ./model
attr = SM.attr

App.Invoice = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr()
  totalCharge: attr('number', as: 'total_charge')
  totalChargeDollars: attr('number', as: 'total_charge_dollars')
  periodStart: attr('date', as: 'period_start')
  periodEnd: attr('date', as: 'period_end')
  successfulCharge: attr('boolean', as:'successful_charge')

  _items: attr('object', as: 'items')

  unpaid: (->
    !@get('successfulCharge')
  ).property 'successfulCharge'

  items: (->
    @get('_items').map (item)->
      inv = App.InvoiceItem.create()
      inv.load item
      inv
    .sortBy('ranking', 'id').reverse()
  ).property '_items'

klass.reopenClass
  url: '/billing/invoices'
  primaryKey: 'id'
  rootKey: 'invoice'
  collectionKey: 'invoices'

#= require ./model
attr = SM.attr

App.InvoiceItem = App.Model.extend
  id: attr('number', serialize: false)
  invoiceId: attr()
  hostId: attr()
  hostSlug: attr()
  hours: attr()
  hourlyCents: attr('number', as: 'hourly_cents')
  totalCharge: attr('number', as: 'total_charge')
  totalChargeDollars: attr('number', as: 'total_charge_dollars')
  description: attr()
  terminated: attr('boolean', as: 'terminated')

  ranking: (->
    if @get 'terminated'
      -1
    else
      0
  ).property('terminated')

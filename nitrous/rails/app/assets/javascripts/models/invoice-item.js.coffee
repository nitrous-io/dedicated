#= require ./model
attr = SM.attr

App.InvoiceItem = App.Model.extend
  id: attr('number', serialize: false)
  invoiceId: attr()
  hostId: attr(as: 'host_id')
  hostSlug: attr(as: 'host_slug')
  hours: attr('number', as: 'hours')
  hourlyCents: attr('number', as: 'hourly_cents')
  totalCharge: attr('number', as: 'total_charge')
  description: attr()
  terminated: attr('boolean', as: 'terminated')
  itemType: attr(as: 'item_type')

  _usage: attr('array', as: 'usage')

  usage: (->
    return [] unless @get("_usage")
    @get('_usage').map (item)=>
      inv = App.HostUsage.create()
      inv.load item
      inv
    .sortBy('startsAt:asc')
  ).property 'usage'

  ranking: (->
    if @get 'terminated'
      -1
    else
      0
  ).property('terminated')

  isCompute: (->
    @get('itemType') == 'compute'
  ).property('itemType')

  totalChargeDollars: (->
    @get('totalCharge') / 100.0
  ).property 'totalCharge'
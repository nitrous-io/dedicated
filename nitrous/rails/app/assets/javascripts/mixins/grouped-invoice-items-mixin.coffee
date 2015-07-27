App.GroupedInvoiceItem = Em.Object.extend
  hostId: null
  items: []

  ranking: (->
    @get('items.lastObject.ranking')
  ).property 'items.@each'

  id: (->
    @get('items.lastObject.ranking')
  ).property 'items.@each'

App.GroupedInvoiceItemsMixin = Em.Mixin.create
  groupedItems: (->
    set = []
    @get('_items')?.forEach (item)=>
      inv = App.InvoiceItem.create()
      inv.load item

      existing = set.findBy('hostId', inv.get('hostId'))
      if existing
        if inv.get 'isCompute'
          existing.get('items').push inv
        else
          existing.get('items').unshift inv
      else
        existing = App.GroupedInvoiceItem.create
          hostId: inv.get('hostId')
          items: [inv]

        set.push existing

    set.sortBy('hostId', 'ranking', 'id').reverse()
  ).property '_items'

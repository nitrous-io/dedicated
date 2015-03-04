#= require ./model
attr = SM.attr

App.BillingUsage = klass = App.Model.extend
  totalCharge: attr('number', as: 'total_charge')
  availableCreditsDollars: attr('number', as: 'available_credits_dollars')

  _items: attr('object', as: 'items')
  items: (->
    @get('_items').map (item)->
      inv = App.InvoiceItem.create()
      inv.load item
      inv
    .sortBy('ranking', 'id').reverse()
  ).property '_items'

  totalChargeDollars: (->
    @get('totalCharge') / 100.0
  ).property 'totalCharge'

  unpaid: (->
    !@get('successfulCharge')
  ).property 'successfulCharge'

  hasCredit: (->
    @get('availableCreditsDollars') > 0
  ).property 'availableCreditsDollars'

  estimatedCharge: (->
    credits = @get 'availableCreditsDollars'
    total = @get 'totalChargeDollars'
    if credits > total
      return 0
    else
      total - credits
  ).property 'availableCreditsDollars', 'totalChargeDollars'

  creditsRemaining: (->
    credits = @get 'availableCreditsDollars'
    total = @get 'totalChargeDollars'
    if credits > total
      return credits - total
    else
      return 0

  ).property 'availableCreditsDollars', 'totalChargeDollars'

klass.reopenClass
  fetchCurrentUsage: ->
    adapter = Ember.get(this, 'adapter');
    adapter.ajax('GET', '/billing/current_usage').then (data) =>
      inst = App.BillingUsage.create()
      inst.load data
      App.set 'billingUsage', inst
      inst

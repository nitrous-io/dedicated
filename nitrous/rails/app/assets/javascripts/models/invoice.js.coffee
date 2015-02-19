#= require ./model
attr = SM.attr

App.Invoice = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr()
  totalCharge: attr('number', as: 'total_charge')
  periodStart: attr('date', as: 'period_start')
  periodEnd: attr('date', as: 'period_end')
  successfulCharge: attr('boolean', as:'successful_charge')
  totalCredit: attr('number', as: 'total_credit')
  creditAvailable: attr('number', as: 'available_credit')

  hasCredit: Ember.computed.gt('totalCredit', 0)

  _items: attr('object', as: 'items')

  unpaid: (->
    !@get('successfulCharge')
  ).property 'successfulCharge'

  totalUsageForPeriod: (->
    @get('totalCreditDollars') + @get('totalChargeDollars')
  ).property 'totalCreditDollars', 'totalChargeDollars'

  creditAvailableDollars: (->
    @get('creditAvailable') / 100.0
  ).property 'creditAvailable'

  totalCreditDollars: (->
    @get('totalCredit') / 100.0
  ).property 'totalCreditDollars'

  totalChargeDollars: (->
    @get('totalCharge') / 100.0
  ).property 'totalCharge'

  items: (->
    @get('_items').map (item)->
      inv = App.InvoiceItem.create()
      inv.load item
      inv
    .sortBy('ranking', 'id').reverse()
  ).property '_items'

  pay: ->
    new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'POST',
        url: "/api/v0/invoices/#{@get 'id'}/pay",
        headers: { Authorization: "Bearer #{App.get 'token'}" },
      ).done((retData, status, jqxhr) =>
        resolve(retData)
      ).fail (jqxhr) ->
        reject(jqxhr)

klass.reopenClass
  url: '/billing/invoices'
  primaryKey: 'id'
  rootKey: 'invoice'
  collectionKey: 'invoices'

#= require ./model
#= require ../mixins/grouped-invoice-items-mixin
attr = SM.attr

App.Invoice = klass = App.Model.extend App.GroupedInvoiceItemsMixin,
  id: attr('number', serialize: false)
  userId: attr()
  totalCharge: attr('number', as: 'total_charge')
  periodStart: attr('date', as: 'period_start')
  periodEnd: attr('date', as: 'period_end')
  state: attr()
  attempted: attr('boolean', as: 'attempted')
  totalCredit: attr('number', as: 'total_credit')
  creditAvailable: attr('number', as: 'available_credit')
  subscription: attr('boolean')

  hasCredit: Ember.computed.gt('totalCredit', 0)

  _items: attr('object', as: 'items')

  awaitingUpdate: false
  paying: false

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

  deferred: (->
    @get('state') == 'deferred'
  ).property 'state'

  successfulCharge: (->
    @get('state') == 'paid'
  ).property 'state'

  failed: (->
    @get('state') == 'failed'
  ).property 'state'

  payable: (->
    @get('state') != 'not_payable'
  ).property 'state'

  showInvoice: (->
    return true if !App.get('billingProfile.exempt')
    @get('payable')
  ).property 'payable', 'App.billingProfile.exempt'

  showInvoiceState: (->
    @get('payable') && !App.get('billingProfile.exempt')
  ).property 'payable', 'App.billingProfile.exempt'

  manuallyPayable: (->
    return false if @get('successfulCharge') || @get('deferred')
    @get('failed') || @get('attempted')
  ).property 'failed', 'successfulCharge', 'attempted'

  showSubscriptionEndHeading: (->
    return false unless @get 'subscription'
    App.get('subscriptions.length') <= 1
  ).property 'App.subscriptions.@each', 'subscription'

  pay: ->
    new Em.RSVP.Promise (resolve, reject) =>
      @set 'paying', true
      Em.$.ajax(
        type: 'POST',
        url: "/api/v0/invoices/#{@get 'id'}/pay",
        headers: { Authorization: "Bearer #{App.get 'token'}" },
      ).done((retData, status, jqxhr) =>
        @set 'paying', false
        resolve(retData)
      ).fail (jqxhr) =>
        @set 'paying', false
        reject(jqxhr)

klass.reopenClass
  url: '/billing/invoices'
  primaryKey: 'id'
  rootKey: 'invoice'
  collectionKey: 'invoices'

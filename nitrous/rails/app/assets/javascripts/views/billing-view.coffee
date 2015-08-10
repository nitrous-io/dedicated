App.BillingView = Em.View.extend
  elementId: 'billing-view'
  classNames: ['ui', 'card']

  didClickShowInvoices: ->
    $("body").scrollTop($("#invoices-list").offset().top)

  showSubscriptions: (->
    return true if App.get('subscriptions.length') > 0
    return true if App.get('billingUsage.groupedItems.length') == 0
    false
  ).property 'App.subscriptions.length', 'App.billingUsage.groupedItems.length'

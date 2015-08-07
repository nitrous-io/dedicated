App.BillingView = Em.View.extend
  elementId: 'billing-view'
  classNames: ['ui', 'card']

  didInsertElement: ->
    if cardType = App.get('billingProfile.cardType')?.underscore()
      if cardType == 'american_express'
        cardType = 'amex'
      else if cardType == 'master_card'
        cardType = 'mastercard'
      @$('.card-type').css 'background-image', "url(#{location.protocol}//nitrous-image-icons.s3.amazonaws.com/cards/#{cardType}.png)"

  didClickShowInvoices: ->
    $("body").scrollTop($("#invoices-list").offset().top)

  showSubscriptions: (->
    return true if App.get('subscriptions.length') > 0
    return true if App.get('billingUsage.groupedItems.length') == 0
    false
  ).property 'App.subscriptions.length', 'App.billingUsage.groupedItems.length'

  hasSingleSubscription: (->
    App.get('subscriptions.length') == 1
  ).property 'App.subscriptions.length'

  hasNoSubscriptions: (->
    App.get('subscriptions.length') == 0
  ).property 'App.subscriptions.length'

  multipleSubscriptions: (->
    App.get('subscriptions.length') > 1
  ).property 'App.subscriptions.length'


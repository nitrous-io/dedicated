App.BillingView = Em.View.extend
  elementId: 'billing-view'
  classNames: ['ui', 'card']

  didInsertElement: ->
    if cardType = App.get('billingProfile.cardType')?.underscore()
      if cardType == 'american_express'
        cardType = 'amex'
      else if cardType == 'master_card'
        cardType = 'mastercard'
      $('#card-type').css 'background-image', "url(#{location.protocol}//nitrous-image-icons.s3.amazonaws.com/cards/#{cardType}.png)"

  didClickShowInvoices: ->
    $("body").scrollTop($("#invoices-list").offset().top)


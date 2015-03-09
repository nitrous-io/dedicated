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

  actions:
    toggleItems: ->
      related = $(event.target).parents(".title").siblings(".items")
      if related.hasClass("show")
        related.removeClass("flow")
        related.removeClass("show")
      else
        related.addClass("show")
        window.setTimeout(->
          related.addClass("flow")
         , 500)

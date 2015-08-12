App.BillingCardInfoComponent = Em.Component.extend
  layoutName: 'components/billing/card-info'
  elementId: 'billing-card-info'

  billingProfile: null

  cardImageCss: (->
    if cardType = @get('billingProfile.cardType')?.underscore()
      if cardType == 'american_express'
        cardType = 'amex'
      else if cardType == 'master_card'
        cardType = 'mastercard'
      "background-image: url(#{location.protocol}//nitrous-image-icons.s3.amazonaws.com/cards/#{cardType}.png)"
  ).property 'billingProfile.cardType'

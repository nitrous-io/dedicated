App.CreditCardView = Em.View.extend
  templateName: 'templates/n2o/credit-cards/credit-card'
  elementId: 'credit_card_view'

  card: null

  didInsertElement: ->
    # make form fields behave as payment fields
    $('#card_number').payment 'formatCardNumber'
    $('#card_expiration').payment 'formatCardExpiry'
    $('#card_cvc').payment 'formatCardCVC'

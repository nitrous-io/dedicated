App.CreditCard = Em.Object.extend
  number: null
  cvc: null
  billingZip: null
  name: null

  expirationMonth: null
  expirationYear: null

  errors: null

  reset: ->
    @set 'number', null
    @set 'cvc', null
    @set 'expirationMonth', null
    @set 'expirationYear', null
    @set 'billingZip', null
    @set 'errors', null
    @set 'name', null

  expiration: ( (key, value) ->
    if arguments.length > 1
      {month, year} = $.payment.cardExpiryVal value
      @set 'expirationMonth', month
      @set 'expirationYear', year
    else
      expirationMonth = @get 'expirationMonth'
      expirationYear = @get 'expirationYear'

      if expirationMonth && expirationYear
        "#{expirationMonth}/#{expirationYear}"
  ).property 'expirationMonth', 'expirationYear'

  stripeParams: (->
    number: @get('number')
    cvc: @get('cvc')
    exp_month: @get('expirationMonth')
    exp_year: @get('expirationYear')
    address_zip: @get('billingZip')
    name: @get('name')
  ).property 'number', 'cvc', 'expirationMonth', 'expirationYear', 'billingZip', 'name'

  valid: ->
    errors = {}

    number = @get 'number'
    unless number && $.payment.validateCardNumber(number)
      errors.number = true

    expirationMonth = @get 'expirationMonth'
    expirationYear = @get 'expirationYear'
    unless expirationMonth && expirationYear && $.payment.validateCardExpiry(expirationMonth, expirationYear)
      errors.expiration = true

    cvc = @get 'cvc'
    cardType = $.payment.cardType(number)
    unless cvc && $.payment.validateCardCVC(cvc, cardType)
      errors.cvc = true

    errors.name = true unless @get('name')

    @set 'errors', errors

    return _.isEmpty(errors)

  createToken: (callback) ->
    if this.valid()
      Stripe.card.createToken @get('stripeParams'), (status, response) =>
        if response.error
          callback response.error.message
        else
          callback null, response.id
    else
      callback 'The credit card details you entered were invalid.'

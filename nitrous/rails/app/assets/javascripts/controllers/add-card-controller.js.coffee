#= require jquery.payment

App.AddCardController = Em.ObjectController.extend
  name: null
  number: null
  expMonth: null
  expYear: null
  cvc: null
  zipcode: null

  expiration: null
  saving: false
  invalidField: null

  clear: ->
    @_super.apply this, arguments
    @setProperties
      name: null
      number: null
      expiration: null
      expMonth: null
      expYear: null
      cvc: null
      zipcode: null
      saving: false
      invalidField: null

  valid: ->
    if !$.payment.validateCardNumber @get('number')
      @set 'invalidField', 'number'
      return false
    if !$.payment.validateCardExpiry @get('expMonth'), @get('expYear')
      @set 'invalidField', 'expiration'
      return false
    if !$.payment.validateCardCVC @get('cvc')
      @set 'invalidField', 'cvc'
      return false
    true

  actions:
    save: ->
      @set 'saving', true
      exp = $.payment.cardExpiryVal @get('expiration')
      @set 'expMonth', exp.month
      @set 'expYear', exp.year
      if !@valid()
        @set 'saving', false
      else
        App.get('billingProfile').addCard(
          number: @get('number'),
          exp_month: @get('expMonth'),
          exp_year: @get('expYear'),
          cvc: @get('cvc')
        ).then =>
          @set 'saving', false
          @transitionToRoute '/'
        , (errorMsg) =>
          @set 'saving', false
          App.postError(errorMsg, 'Failed to create billing profile!')

    cancel: ->
      @transitionToRoute 'billing'

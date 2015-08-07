App.BillingAddCardComponent = Em.Component.extend
  layoutName: 'components/billing/add-card'
  elementId: 'billing-add-card'
  classNames: ['ui', 'card']

  billingProfile: null
  subscriptionSlug: null
  pendingPlanId: null
  pendingHost: null
  pendingContainerData: null

  name: null
  number: null
  expMonth: null
  expYear: null
  cvc: null
  zipcode: null

  expiration: null
  saving: false
  invalidField: null

  numberFieldInvalid: Em.computed.equal('invalidField', 'credit card number')
  expirationFieldInvalid: Em.computed.equal('invalidField', 'expiration')
  cvcFieldInvalid: Em.computed.equal('invalidField', 'cvc')
  zipFieldInvalid: Em.computed.equal('invalidField', 'zip')

  clear: (->
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
  ).observes 'billingProfile'

  saveButtonDisabled: (->
    !(@get('name') && @get('number') && @get('expiration') && @get('cvc')) || @get('saving')
  ).property 'name', 'number', 'expiration', 'cvc', 'saving'

  showCardIcon: (->
    backgroundImageURL = "none"
    if cardType = $.payment.cardType @get('number')
      backgroundImageURL = "url(#{location.protocol}//nitrous-image-icons.s3.amazonaws.com/cards/#{cardType}.png)"
    @$('#card-number').css 'background-image', backgroundImageURL
  ).observes 'number'

  pendingPlan: (->
    App.get('plans').findBy('id', parseInt(@get('pendingPlanId')))
  ).property 'pendingPlanId'

  isValid: ->
    if !$.payment.validateCardNumber @get('number')
      @set 'invalidField', 'credit card number'
      return false
    if !$.payment.validateCardExpiry @get('expMonth'), @get('expYear')
      @set 'invalidField', 'expiration'
      return false
    if !$.payment.validateCardCVC @get('cvc')
      @set 'invalidField', 'cvc'
      return false
    true

  processErrorMessage: (error)->
    switch error.error
      when 'incorrect_cvc'
        @set 'invalidField', 'cvc'
      when 'incorrect_zip'
        @set 'invalidField', 'zipcode'
      when 'card_declined', 'card_expired'
        @set 'invalidField', 'credit card number'

  didInsertElement: ->
    @clear()
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('#card-number').payment 'formatCardNumber'
      @$('#card-expiration').payment 'formatCardExpiry'
      @$('#card-cvc').payment 'formatCardCVC'
      @$('[autofocus]').focus()

  actions:
    showHelp: ->
      App.set 'onboardingScreen', 'cardholders-name'

    save: ->
      # jquery.payments fails to notify (change()) when it reformats
      # causing Ember to not know about a changed attribute, we force an update
      @set 'expiration', @$('#card-expiration').val()
      @set 'invalidField', null
      exp = $.payment.cardExpiryVal @get('expiration')
      @set 'expMonth', exp.month
      @set 'expYear', exp.year
      if !@isValid()
        App.postError "Invalid #{@get 'invalidField'}"

      else
        @set 'saving', true

        billingProfile = @get 'billingProfile'

        hadCard = billingProfile.get('hasCard')

        billingProfile.addCard(
          name: @get('name'),
          number: @get('number'),
          exp_month: @get('expMonth'),
          exp_year: @get('expYear'),
          cvc: @get('cvc'),
          zipcode: @get('zipcode')
        ).then (bp) =>
          @set 'saving', false
          App.refreshUsage()
          App.refreshBillingProfile()

          if hadCard
            App.snackbar.post "Credit card updated."
          else
            App.snackbar.post "Credit card added."

          if (sub = @get 'subscriptionSlug') && (pl = @get 'pendingPlanId')
            PubSub.publish 'billing.addCard.upgradedToPaid', subscriptionSlug: sub, planId: pl
          else
            PubSub.publish 'billing.addCard.added', @get('pendingHost')?
        , (errorMsg) =>
          @set 'saving', false
          @processErrorMessage(errorMsg)
          App.postError(errorMsg, 'Failed to create billing profile!')

    cancel: ->
      PubSub.publish 'billing.addCard.canceled', @get('pendingHost')?

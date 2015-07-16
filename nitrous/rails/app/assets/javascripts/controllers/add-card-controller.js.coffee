#= require jquery.payment

App.AddCardController = Em.ObjectController.extend
  queryParams: ['upgradingFromFree', 'hostSlug', 'subscriptionId', 'pendingPlanId']
  # To indicate whether user is upgrading from free tier.
  upgradingFromFree: false
  # Slug of the host that we are upgrading.
  hostSlug: null

  # when upgrading a subscription
  subscriptionId: null
  pendingPlanId: null

  name: null
  number: null
  expMonth: null
  expYear: null
  cvc: null
  zipcode: null

  expiration: null
  saving: false
  invalidField: null

  pendingPlan: (->
    App.get('plans').findBy('id', parseInt(@get('pendingPlanId')))
  ).property 'pendingPlanId'

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

  actions:
    save: ->
      # jquery.payments fails to notify (change()) when it reformats
      # causing Ember to not know about a changed attribute, we force an update
      @set 'expiration', Em.$('#card-expiration').val()
      @set 'invalidField', null
      exp = $.payment.cardExpiryVal @get('expiration')
      @set 'expMonth', exp.month
      @set 'expYear', exp.year
      if !@valid()
        App.postError "Invalid #{@get 'invalidField'}"

      else
        @set 'saving', true

        billingProfile = App.get('billingProfile')
        wasFreeTier = billingProfile.get('free')
        anyHosts = App.get('hosts.length') > 0

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

          if (sub = @get 'subscriptionId') && (pl = @get 'pendingPlanId')
            @set 'subscriptionId', null
            @set 'pendingPlanId', null
            @transitionToRoute 'subscriptions.upgrade', sub, queryParams: { pendingPlanId: pl }
            return

          if wasFreeTier && anyHosts
            if credit = bp.get('amountCreditedDollars')
              App.set 'upgradeCredited', credit
            @transitionToRoute 'host.index', @get('hostSlug'), queryParams: { justUpgraded: true }
            return

          App.snackbar.post "Credit card added."

          if App.get('pendingHost')
            @transitionToRoute 'hosts.new'
          else
            @transitionToRoute 'billing'

        , (errorMsg) =>
          @set 'saving', false
          @processErrorMessage(errorMsg)
          App.postError(errorMsg, 'Failed to create billing profile!')

    cancel: ->
      if App.get('pendingHost')
        App.set('pendingHost', null)
        @transitionToRoute 'hosts.new'
      else
        @transitionToRoute 'billing'

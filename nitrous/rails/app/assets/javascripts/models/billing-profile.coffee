#= require ./model
attr = SM.attr

App.BillingProfile = klass = App.Model.extend
  id: attr('number', serialize: false)
  state: attr()
  userId: attr('number', as: 'user_id', serialize: false)
  orgId: attr('number', as: 'org_id', serialize: false)
  lastFour: attr(as: 'last_four', serialize: false)
  cardType: attr(as: 'card_type', serialize: false)
  expMonth: attr(as: 'exp_month', serialize: false)
  expYear: attr(as: 'exp_year', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'created_at', serialize: false)
  cardRequired: attr('boolean', as: 'card_required', serialize: false)
  amountCredited: attr('number', as: 'amount_credited', serialize: false)
  trialEligible: attr('boolean', as: 'trial_eligible', serialize: false)

  cardToken: attr(as: 'card_token')

  paid: (->
    @get('state') == 'paid' || @get('state') == 'payment_due'
  ).property 'state'

  exempt: (->
    @get('state') == 'exempt'
  ).property 'state'

  free: (->
    @get('state') == 'free'
  ).property 'state'

  active: (->
    @get('paid') || @get('exempt') || @get('free')
  ).property 'paid', 'exempt', 'free'

  expiry: (->
    "#{@get 'expMonth'}/#{@get 'expYear'}"
  ).property 'expMonth', 'expYear'

  hasCard: (->
    @get('state') != 'no_card' && @get('lastFour').length > 0
  ).property 'state', 'lastFour'

  requiresAttention: (->
    @get('state') == 'payment_due' || @get('state') == 'delinquent'
  ).property 'state'

  isNoCardState: (->
    @get('state') == 'no_card'
  ).property('state')

  isFreeOrNoCardState: (->
    @get('state') == 'no_card' || @get('state') == 'free'
  ).property('state')

  amountCreditedDollars: (->
    return unless (am = @get('amountCredited')) && am > 0
    parseFloat(@get('amountCredited')) / 100.0
  ).property 'amountCredited'

  creditForPaidPlan: (->
    return unless @get('free')
    @get 'amountCreditedDollars'
  ).property 'amountCreditedDollars', 'free'

  # override the default save method
  save: (params, options) ->
    adapter = Ember.get(@constructor, 'adapter')
    if @get('hasCard')
      adapter.updateRecord(this, params, options)
    else
      adapter.createRecord(this, params, options)

  addCard: (params) ->
    new Ember.RSVP.Promise (resolve, reject) =>
      Stripe.card.createToken {
        name: params.name,
        number: params.number,
        exp_month: params.exp_month,
        exp_year: params.exp_year,
        cvc: params.cvc,
        address_zip: params.zipcode
      }, (status, response) =>
        if response.error
          return reject(response.error.message)
        @set 'cardToken', response.id
        @save().then  (d)=>
          @set 'cardToken', null
          resolve(d)
        , (jqxhr) =>
          reject(jqxhr.responseJSON)

klass.reopenClass
  url: '/user/billing_profile'
  singular: true
  rootKey: 'billing_profile'

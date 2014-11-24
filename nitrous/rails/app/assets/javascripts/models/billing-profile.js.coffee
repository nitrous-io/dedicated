#= require ./model
attr = SM.attr

App.BillingProfile = klass = App.Model.extend
  id: attr('number', serialize: false)
  state: attr()
  userId: attr('number', as: 'user_id', serialize: false)
  lastFour: attr(as: 'last_four', serialize: false)
  cardType: attr(as: 'card_type', serialize: false)
  expMonth: attr(as: 'exp_month', serialize: false)
  expYear: attr(as: 'exp_year', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'created_at', serialize: false)

  cardToken: attr(as: 'card_token')

  paid: (->
    @get('state') == 'paid' 
  ).property 'state'

  expiry: (->
    "#{@get 'expMonth'}/#{@get 'expYear'}"
  ).property 'expMonth', 'expYear'

  addCard: (params) ->
    new Ember.RSVP.Promise (resolve, reject) =>
      Stripe.card.createToken {
        number: params.number,
        exp_month: params.exp_month,
        exp_year: params.exp_year,
        cvc: params.cvc
      }, (status, response) =>
        if response.error
          return reject(response.error.message)
        @set 'cardToken', response.id
        @save().then  =>
          @set 'cardToken', null
          resolve()
        , (jqxhr) =>
          reject(jqxhr.responseJSON)

klass.reopenClass
  url: '/user/billing_profile'
  singluar: true
  rootKey: 'billing_profile'

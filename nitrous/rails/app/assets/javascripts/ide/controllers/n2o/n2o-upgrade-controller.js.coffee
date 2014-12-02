App.N2oUpgradeController = Em.Controller.extend
  subscriptionBinding: 'App.subscription'

  card: null
  currentPlan: null
  selectedPlan: null
  stripeToken: null

  processing: false
  processingErrorMessage: null
  confirmUpgradeDialog: null

  initialize: ->
    @set 'card', App.CreditCard.create()
    @set 'selectedPlan', App.Plan.personalPlans.findProperty('highlight', true)

  reset: ->
    @set 'processing', false
    @set 'processingErrorMessage', null
    @set 'currentPlan', null
    @set 'confirmUpgradeDialog', null
    @set 'stripeToken', null

  discount: (->
    coupon = App.currentUser.get('coupon')
    return 0 unless coupon

    percentOff = coupon.percent_off
    selectedPlanPrice = @get('selectedPlan').get('price')

    selectedPlanPrice * ( percentOff / 100 )
  ).property 'App.currentUser.coupon', 'selectedPlan'

  total: (->
    selectedPlanPrice = @get('selectedPlan').get('price')
    selectedPlanPrice - @get('discount')
  ).property 'selectedPlan', 'discount'

  displayN2OInfo: (->
    return App.get('boxes.length') == 0
  ).property 'App.boxes.length'

  upgrade: ->
    @set 'processing', true
    @set 'processingErrorMessage', null
    card = @get 'card'

    unless selectedPlan = @get 'selectedPlan'
      return

    card.createToken (error, stripeToken) =>
      if error
        @set 'processing', false
        this._setFormError error
      else
        @set 'stripeToken', stripeToken
        @set 'confirmUpgradeDialog', App.ConfirmUpgradeDialog.create plan: selectedPlan, total: @get('total')

  upgradeConfirmed: ->
    App.set 'currentUser.subscription_pending', true
    App.Subscription.subscribe @get('selectedPlan').get('id'), @get('stripeToken'), (error) => this._afterUpgrade(error)

  upgradeCanceled: ->
    @set 'processing', false
    @set 'stripeToken', null

  _afterUpgrade: (error) ->
    @set 'processing', false
    @set 'stripeToken', null
    if error
      this._setFormError error
      return

    @set 'processingErrorMessage', null
    Em.run.scheduleOnce 'afterRender', this, ->
      App.set 'flashMessage', { type: 'alert-success', text: 'We are processing your purchase, you should be receiving an email shortly with a receipt of purchase.' }

    if App.get('currentUser.intro_seen')
      App.get('router').transitionTo 'n2o.bonus'
    else
      App.get('router').transitionTo 'intro.index'

  _setFormError: (error) ->
    @set 'processingErrorMessage', error

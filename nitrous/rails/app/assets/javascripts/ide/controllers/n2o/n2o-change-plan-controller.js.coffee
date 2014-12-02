App.N2oChangePlanController = Em.Controller.extend
  subscriptionBinding: 'App.subscription'

  selectedPlan: null
  currentPlan: null

  processing: false
  processingErrorMessage: null
  changeCreditCardDialog: null

  card: null

  initialize: ->
    plan = @get('subscription').get('plan')
    @set 'selectedPlan', plan
    @set 'currentPlan', plan

    if App.get('entryCouponCode')
      App.set 'flashMessage', type: 'alert-error', text: "Coupons can only be used by new users who haven't already subscribed to a plan."
      App.set('entryCouponCode', null)

  reset: ->
    @set 'processing', false
    @set 'processingErrorMessage', null

  changePlan: (callback) ->
    @set 'processing', true
    @set 'processingErrorMessage', null

    if selectedPlan = @get 'selectedPlan'
      return @get('subscription').changePlan selectedPlan.get('id'), (error) => this._afterUpgrade(error)

  changeCreditCard: ->
    @set 'card', App.CreditCard.create()
    @set 'changeCreditCardDialog', App.ChangeCreditCardDialog.create(card: @get('card'))

  _afterUpgrade: (error) ->
    @set 'processing', false
    if error
      this._setFormError error
    else
      App.set 'currentUser.subscription_pending', true
      this._redirectToBonusPage()

  _setFormError: (error) ->
    @set 'processingErrorMessage', error

  _redirectToBonusPage: ->
    @set 'processingErrorMessage', null
    Em.run.scheduleOnce 'afterRender', this, '_showFlashMessage'
    App.get('router').transitionTo 'n2o.bonus'

  _showFlashMessage: ->
    App.set 'flashMessage', { type: 'alert-success', text: 'We are processing your purchase, you should be receiving an email shortly with a receipt of purchase.' }

App.AccountController = Em.ObjectController.extend
  content: null
  resettingPassword: false
  cancelingSubscription: false

  changeCreditCardDialog: null
  confirmCancelSubscriptionDialog: null
  confirmDeleteAccountDialog: null

  card: null
  receipts: null

  hasActiveOrDelinquentSubscription: (->
    if subscription = App.get 'subscription'
      state = subscription.get('state')
      ['active', 'delinquent'].indexOf(state) > -1
  ).property 'App.subscription.state'

  currentPlan: (->
    if subscription = App.get 'subscription'
      subscription.get('plan').get('name')
  ).property 'App.subscription.plan'

  currentCardDetails: (->
    if billingProfile = App.get 'billingProfile'
      formattedCardNumber = billingProfile.get('formatted_card_number')
      cardType = billingProfile.get('card_type')
      "#{formattedCardNumber} (#{cardType})"
  ).property 'App.billingProfile.formatted_card_number', 'App.billingProfile.card_type'

  resetPassword: ->
    @set 'resettingPassword', true
    record = @get 'content'
    record.resetPassword()
          .done =>
            App.set 'flashMessage', { type: 'alert-info', text: 'Check your email for instructions on resetting your password.' }
          .always =>
            @set 'resettingPassword', false

  updateProfile: ->
    record = @get 'content'
    record.set 'errors', null
    record.updateInfo()
          .done ->
            App.set 'flashMessage', { type: 'alert-success', text: 'Your account details were updated successfully.' }

  changeCreditCard: ->
    @set 'card', App.CreditCard.create()
    @set 'changeCreditCardDialog', App.ChangeCreditCardDialog.create(card: @get('card'))

  confirmCancelSubscription: ->
    @set 'confirmCancelSubscriptionDialog', App.ConfirmCancelSubscriptionDialog.create()

  cancelSubscription: ->
    subscription = App.get 'subscription'
    @set 'cancelingSubscription', true
    subscription.cancel (error) =>
      @set 'cancelingSubscription', false
      if error
        App.set 'flashMessage', { type: 'alert-error', text: error }
      else
        subscription.set 'state', 'canceled'
        App.set 'flashMessage', { type: 'alert-info', text: 'Subscription canceled.' }
      App.util.mixpanelAppendUser "Cancelled Subscription": new Date() + (if error then " failed" else " success")

  confirmDeleteAccount: ->
    @set 'confirmDeleteAccountDialog', App.ConfirmDeleteAccountDialog.create()

  fetchReceipts: ->
    App.Receipt.fetch().then (receipts) =>
      @set 'receipts', receipts

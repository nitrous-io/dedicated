App.AccountView = Em.View.extend
  templateName: 'templates/account/account'
  classNames: ['app_page', 'index']
  elementId: 'account-view'

  resettingPassword: false

  hasPaidSubscription: (->
    App.get('subscription')?.get('active')
  ).property 'App.subscription.active'

  hasVisibleBoxes: (->
    App.get('boxes.length') > 0
  ).property 'App.boxes.length'

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Account Settings Page'

  didClickPasswordReset: ->
    @get('controller').resetPassword()

  didClickUpdate: ->
    @get('controller').updateProfile()

  didClickChangeCreditCard: ->
    @get('controller').changeCreditCard()

  didClickCancelSubscription: ->
    @get('controller').confirmCancelSubscription()

  didClickChangePlan: ->
    App.get('router').transitionTo 'n2o.change_plan'

  didClickDeleteAccount: ->
    if @get 'hasPaidSubscription'
      App.set 'flashMessage', { type: 'alert-error', text: 'You must cancel your subscription before deleting your account.' }
    else if @get 'hasVisibleBoxes'
      App.set 'flashMessage', { type: 'alert-error', text: 'You must terminate all your boxes before deleting your account.' }
    else
      @get('controller').confirmDeleteAccount()

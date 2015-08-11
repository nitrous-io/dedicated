App.BillingController = Em.ObjectController.extend
  subscription: null

  personalSubscriptions: (->
    App.get('subscriptions').filter (subscription) ->
      !subscription.get('orgId')
  ).property 'App.subscriptions.@each'

  actions:
    changeCard: ->
      @transitionTo 'add-card'

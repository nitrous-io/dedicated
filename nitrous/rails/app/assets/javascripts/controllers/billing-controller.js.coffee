#= require jquery.payment

App.BillingController = Em.ObjectController.extend
  actions:
    changeCard: ->
      @transitionTo 'add-card'

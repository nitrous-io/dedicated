#= require jquery.payment

App.BillingController = Em.ObjectController.extend
  actions:
    changeCard: ->
      @transitionToRoute 'add-card'

App.DelinquentMessageView = Em.View.extend
  templateName: 'templates/partials/delinquent-message'

  didClickChangeCreditCard: ->
    App.get('router').transitionTo 'account'
    Em.run.scheduleOnce 'afterRender', this, ->
      App.get('router.accountController').changeCreditCard()

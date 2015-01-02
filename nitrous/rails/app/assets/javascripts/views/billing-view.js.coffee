App.BillingView = Em.View.extend
  elementId: 'billing-view'
  classNames: ['ui', 'card']

  didInsertElement: ->
    if cardType = App.get('billingProfile.cardType')?.underscore()
      $('#card-type').css 'background-image', "url(assets/cards/#{cardType}_32.png)"

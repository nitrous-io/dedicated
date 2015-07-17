App.SubscriptionsUpgradeView = Em.View.extend
  elementId: 'subscriptions-upgrade-view'
  classNames: ['ui', 'card']

  actions:
    showUpgradeDialog: ->
      ui.showModal('#subscription-upgrade-modal-dialog')

    cancelUpgrade: ->
      ui.hideModal('#subscription-upgrade-modal-dialog')

    confirmUpgrade: ->
      @get('controller').send 'performUpgrade'

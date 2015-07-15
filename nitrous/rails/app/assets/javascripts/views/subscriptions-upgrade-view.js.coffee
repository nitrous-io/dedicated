App.SubscriptionsUpgradeView = Em.View.extend
  elementId: 'subscriptions-upgrade-view'
  classNames: ['ui', 'card']

  upgradeModalTitle: (->
    return "Resizing" if @get('controller.hostsUpgrading.length') > 0
    return "Upgrading" if @get('controller.upgrading')
    "Begin Upgrade"
  ).property 'controller.hostsUpgrading.@each', 'controller.upgrading'

  actions:
    showUpgradeDialog: ->
      ui.showModal('#subscription-upgrade-modal-dialog')

    cancelUpgrade: ->
      ui.hideModal('#subscription-upgrade-modal-dialog')

    closeUpgrade: ->
      ui.hideModal('#subscription-upgrade-modal-dialog')
      if (host = @get('controller.hostsUpgrading.firstObject'))
        @get('controller').transitionToRoute 'host.index', host.get 'slug'

    confirmUpgrade: ->
      @get('controller').send 'performUpgrade'

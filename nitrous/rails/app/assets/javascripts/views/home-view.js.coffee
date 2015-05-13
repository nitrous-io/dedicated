App.HomeView = Em.View.extend
  elementId: 'home-view'

  showUpgradeModal: (->
    return unless App.get('showUpgradeModal')
    ui.showModal('#upgrade-complete-modal-dialog')
  ).observes 'App.showUpgradeModal'

  actions:
    closeUpgradeModal: ->
      @get('controller').send 'redirectToFirstContainer'
      App.set 'showUpgradeModal', false
      App.set 'upgradeCredited', null
      ui.hideModal('#upgrade-complete-modal-dialog')
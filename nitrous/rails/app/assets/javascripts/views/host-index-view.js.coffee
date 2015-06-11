App.HostIndexView = Em.View.extend
  elementId: 'host-index-view'

  terminateSlugConfirmation: null

  terminateSlugConfirmationInvalid: (->
    @get('controller.model.slug') != @get('terminateSlugConfirmation')
  ).property 'controller.model.slug', 'terminateSlugConfirmation'

  hostStatsAvailable: (->
    @get('controller.model.state') == 'running' && @get('controller.model.stats')
  ).property 'controller.model.state', 'controller.model.stats'

  showUpgradeModal: (->
    if @get('controller.justUpgraded')
      Em.run.later(->
        Em.run.scheduleOnce 'afterRender', ->
          ui.showModal('#upgrade-complete-modal-dialog')
      , 100)
  ).observes('controller.justUpgraded').on('init')

  actions:
    refresh: ->
      $btn = @$('.refresh-btn')
      $btn.attr 'disabled', true
      @get('controller').send 'refresh'
      window.setTimeout(->
        $btn.attr 'disabled', false
      , 1000)

    showTerminateDialog: ->
      @set 'terminateSlugConfirmation', null
      ui.showModal('#terminate-modal-dialog')

    cancelTerminate: ->
      ui.hideModal('#terminate-modal-dialog')

    confirmTerminate: ->
      ui.hideModal('#terminate-modal-dialog')
      @get('controller').send 'terminate'

    showStopDialog: ->
      ui.showModal('#stop-modal-dialog')

    cancelStop: ->
      ui.hideModal('#stop-modal-dialog')

    confirmStop: ->
      ui.hideModal('#stop-modal-dialog')
      @get('controller').send 'stop', force: true

    showRestartDialog: ->
      ui.showModal('#restart-modal-dialog')

    cancelRestart: ->
      ui.hideModal('#restart-modal-dialog')

    confirmRestart: ->
      ui.hideModal('#restart-modal-dialog')
      @get('controller').send 'restart', force: true

    hideUpgradeDialog: ->
      ui.hideModal('#upgrade-complete-modal-dialog')

    respecNow: ->
      @get('controller').send 'respec'
      @send('hideUpgradeDialog')

  clickTerminate: =>
    @$('.terminate-btn').click()

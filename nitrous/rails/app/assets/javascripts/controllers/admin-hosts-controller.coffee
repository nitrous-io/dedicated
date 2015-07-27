App.AdminHostsController = Em.Controller.extend
  unaccountedTerminate: false
  terminateTargetHost: null
  restartTargetHost: null

  actions:
    showTerminateDialog: (host) ->
      @set 'unaccountedTerminate', false
      @set 'terminateTargetHost', host
      @set 'restartTargetHost', null
      ui.showModal('#terminate-modal-dialog')

    cancelTerminate: ->
      @set 'terminateTargetHost', null
      ui.hideModal('#terminate-modal-dialog')

    confirmTerminate: ->
      ui.hideModal('#terminate-modal-dialog')
      this.get('target').send('terminate', @get('terminateTargetHost'))
      @set 'terminateTargetHost', null

    showTerminateUnaccountedDialog: (host) ->
      @set 'unaccountedTerminate', true
      @set 'terminateTargetHost', host
      @set 'restartTargetHost', null
      ui.showModal('#terminate-modal-dialog')

    cancelTerminate: ->
      @set 'terminateTargetHost', null
      ui.hideModal('#terminate-modal-dialog')

    confirmTerminate: ->
      ui.hideModal('#terminate-modal-dialog')
      if @get('unaccountedTerminate')
        this.get('target').send('terminateUnaccounted', @get('terminateTargetHost'))
      else
        this.get('target').send('terminate', @get('terminateTargetHost'))
      @set 'terminateTargetHost', null

    showRestartDialog: (host) ->
      @set 'restartTargetHost', host
      @set 'terminateTargetHost', null
      ui.showModal('#restart-modal-dialog')

    cancelRestart: ->
      @set 'restartTargetHost', null
      ui.hideModal('#restart-modal-dialog')

    confirmRestart: ->
      ui.hideModal('#restart-modal-dialog')
      this.get('target').send('restart', @get('restartTargetHost'), force: true)
      @set 'restartTargetHost', null

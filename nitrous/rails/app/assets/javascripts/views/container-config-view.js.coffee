App.ContainerConfigView = Lib.ContainerConfigView.extend
  elementId: ['container-config-view']

  actions:
    askApply: ->
      ui.showModal(@$('#confirm-apply-modal-dialog'))

    cancelApply: ->
      ui.hideModal(@$('#confirm-apply-modal-dialog'))

    confirmApply: ->
      ui.hideModal(@$('#confirm-apply-modal-dialog'))
      @get('controller').send 'apply'

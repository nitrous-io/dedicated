App.ContainerConfigView = Lib.ContainerConfigView.extend
  elementId: ['container-config-view']

  selectedVolumeEmpty: Em.computed.empty('controller.selectedVolume')
  containerPortEmpty: Em.computed.empty('controller.containerPort')

  addVolumeMountButtonDisabled: Em.computed.or('selectedVolumeEmpty', 'formDisabled')
  addPortMappingButtonDisabled: Em.computed.or('containerPortEmpty', 'formDisabled')
  volumeDropdownDisabled: Em.computed.or('controller.hostVolumesEmpty', 'formDisabled')

  formDisabled: (->
    !@get('controller.loaded') || @get('controller.saving') || !@get('controller.model.canEdit')
  ).property 'controller.loaded', 'controller.saving', 'controller.model.canEdit'

  closeApplyDialogDidChange: (->
    if @get 'controller.closeApplyDialog'
      @set 'controller.closeApplyDialog', false
      @send 'cancelApply'
  ).observes 'controller.closeApplyDialog'

  actions:
    askApply: ->
      ui.showModal(@$('#confirm-apply-modal-dialog'))

    cancelApply: ->
      ui.hideModal(@$('#confirm-apply-modal-dialog'))

    confirmApply: ->
      @get('controller').send 'apply'

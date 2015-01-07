App.ContainerConfigView = Lib.ContainerConfigView.extend App.ApplyDialogMixin,
  elementId: ['container-config-view']

  selectedVolumeEmpty: Em.computed.empty('controller.selectedVolume')
  containerPortEmpty: Em.computed.empty('controller.containerPort')

  addVolumeMountButtonDisabled: Em.computed.or('selectedVolumeEmpty', 'formDisabled')
  addPortBindingButtonDisabled: Em.computed.or('containerPortEmpty', 'formDisabled')
  volumeDropdownDisabled: Em.computed.or('controller.hostVolumesEmpty', 'formDisabled')

  formDisabled: (->
    !@get('controller.loaded') || @get('controller.saving')
  ).property 'controller.loaded', 'controller.saving'

Lib.ContainerConfigView = Em.View.extend
  volumeMountpointInputPattern: '[^\\s\\?\\\\*%:\\|"\'<>$~]+'
  volumeMountpointInputTitle: 'Whitespace and the following characters are not allowed: \\ ? * % : | \" \' < > $ ~'

  addVolumeMountButtonDisabled: Em.computed.empty('controller.selectedVolume')
  addPortMappingButtonDisabled: Em.computed.empty('controller.containerPort')
  volumeDropdownDisabledBinding: Em.Binding.oneWay 'controller.hostVolumesEmpty'

  addEnvVarButtonDisabled: (->
    !(@get('controller.envKey') && @get('controller.envValue'))
  ).property 'controller.envKey', 'controller.envValue'

  hostVolumesDidChange: (->
    Em.run.scheduleOnce 'afterRender', this, ->
      @$('select[name=containers-config-selected-volume]')?.change()
  ).observes 'controller.host.volumes.@each'

App.ContainerPortsView = Em.View.extend
  elementId: 'container-ports-view'

  addDisabled: (->
    controller = @get('controller')
    !controller.get('port') || controller.get('saving')
  ).property 'controller.saving', 'controller.port'

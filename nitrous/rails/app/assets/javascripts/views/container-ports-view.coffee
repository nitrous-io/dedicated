App.ContainerPortsView = Em.View.extend
  elementId: 'container-ports-view'

  formDisabled: (->
    !@get('controller.loaded') || @get('controller.saving')
  ).property 'controller.loaded', 'controller.saving'

  addDisabled: (->
    controller = @get('controller')
    !controller.get('containerPort') || !controller.get('loaded') || controller.get('saving')
  ).property 'controller.containerPort', 'controller.loaded', 'controller.saving'

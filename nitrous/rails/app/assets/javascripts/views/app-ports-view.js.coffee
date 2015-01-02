App.AppPortsView = Em.View.extend
  elementId: 'app-ports-view'

  addDisabled: (->
    controller = @get('controller')
    !controller.get('port') || controller.get('saving')
  ).property 'controller.saving', 'controller.port'

App.AppPortsView = Em.View.extend App.ApplyDialogMixin,
  elementId: 'app-ports-view'

  addDisabled: (->
    controller = @get('controller')
    !controller.get('port') || controller.get('saving')
  ).property 'controller.saving', 'controller.port'

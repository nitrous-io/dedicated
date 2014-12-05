#= require ./container-keys-new-controller

App.AppKeysNewController = App.ContainerKeysNewController.extend
  indexRoute: 'app.keys'
  record: Em.computed.alias 'model.appContainer'

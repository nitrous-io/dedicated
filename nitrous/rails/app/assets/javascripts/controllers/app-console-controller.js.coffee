#= require ./container-console-controller
#
App.AppConsoleController = App.ContainerConsoleController.extend
  freighterdUrl: Em.computed.alias 'model.freighterdUrl'
  record: Em.computed.alias 'model.appContainer'

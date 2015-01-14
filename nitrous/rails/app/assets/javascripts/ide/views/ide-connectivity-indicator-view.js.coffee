App.IdeConnectivityIndicatorView = Em.View.extend
  templateName: 'ide/connectivity-indicator'
  elementId: 'ide-connectivity-indicator-view'
  classNames: ['app-notice-view']
  classNameBindings: ['show']

  show: (->
    App.get('online') && App.get('router.ideController.opsSocketNotReady')
  ).property 'App.online', 'App.router.ideController.opsSocketNotReady'

App.IdeConnectivityIndicatorView = Em.View.extend
  templateName: 'ide/connectivity-indicator'
  elementId: 'ide-connectivity-indicator-view'
  classNames: ['app-notice-view']
  classNameBindings: ['show']

  show: (->
    App.get('online') && App.get('router.indexController.opsSocketNotReady')
  ).property 'App.online', 'App.router.indexController.opsSocketNotReady'

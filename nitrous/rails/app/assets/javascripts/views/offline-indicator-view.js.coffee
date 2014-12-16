App.OfflineIndicatorView = Em.View.extend
  templateName: 'offline-indicator'
  elementId: 'offline-indicator-view'
  classNames: ['app-notice-view']
  classNameBindings: ['App.offline:show']

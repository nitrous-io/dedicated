App.UiConnectivityIndicatorComponent = Em.Component.extend
  classNames: ['ui-connectivity-indicator']
  classNameBindings: ['App.offline:show']

  html: '<strong>Warning:</strong> Your Internet connection appears to be down.'

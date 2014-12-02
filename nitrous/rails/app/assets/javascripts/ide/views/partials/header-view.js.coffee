App.HeaderView = Em.View.extend
  templateName: 'templates/partials/header'
  elementId: 'header_view'

  hideHeaderBinding: Em.Binding.oneWay 'App.isChromeWebView'

  noMenu: false

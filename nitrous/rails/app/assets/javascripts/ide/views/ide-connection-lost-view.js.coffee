App.IdeConnectionLostView = Em.View.extend
  templateName: 'ide/templates/connection-lost'
  elementId: 'ide_connection_lost'

  didClickRetry: ->
    App.util.mixpanel 'Clicked Retry IDE Connection'
    window.location.reload()

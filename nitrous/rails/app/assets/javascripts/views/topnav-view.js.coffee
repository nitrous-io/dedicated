App.TopnavView = Em.View.extend
  elementId: 'topnav-view'

  actions:
    sendFeedback: ->
      App.trigger 'open-feedback-dialog'

    switchUser: ->
      App.trigger 'open-switch-user-dialog'

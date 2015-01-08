App.TopnavView = Em.View.extend
  elementId: 'topnav-view'
  classNames: ['ui', 'topnav']

  newHostTitle: (->
    if !App.get('canCreateHosts')
      maxHosts = App.get('userProfile.maxHosts')
      "You can only create #{maxHosts} workspace#{if maxHosts != 1 then "s" else ""}."
  ).property 'App.canCreateHosts', 'userProfile.maxHosts'

  actions:
    sendFeedback: ->
      App.trigger 'open-feedback-dialog'

    switchUser: ->
      App.trigger 'open-switch-user-dialog'

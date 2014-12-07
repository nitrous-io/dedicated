App.TopnavView = Em.View.extend
  elementId: 'topnav-view'
  classNames: ['ui', 'topnav']

  newApplicationTitle: (->
    if !App.get('canCreateHosts')
      maxHosts = App.get('userProfile.maxHosts')
      if maxHosts == 1
        "You can only create 1 application."
      else
        "You can only create #{maxHosts} applications."
  ).property 'App.canCreateHosts', 'userProfile.maxHosts'

  actions:
    sendFeedback: ->
      App.trigger 'open-feedback-dialog'

    switchUser: ->
      App.trigger 'open-switch-user-dialog'

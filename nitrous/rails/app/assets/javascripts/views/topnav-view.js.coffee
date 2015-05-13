App.TopnavView = Em.View.extend
  elementId: 'topnav-view'
  classNames: ['ui', 'topnav', 'page-header']

  newHostTitle: (->
    if !App.get('canCreateHosts')
      maxHosts = App.get('userProfile.maxHosts')
      "You can only create #{maxHosts} workspace#{if maxHosts != 1 then "s" else ""}."
  ).property 'App.canCreateHosts', 'userProfile.maxHosts'


  didInsertElement: ->
    @_super()

  actions:
    sendFeedback: ->
      App.trigger 'open-feedback-dialog'

    switchUser: ->
      App.trigger 'open-switch-user-dialog'

    creditUser: ->
      App.trigger 'open-credit-user-dialog'

    updateAdminHosts: ->
      App.trigger 'show-update-hosts', App.get('hosts')

    switchBackAdmin: ->
      App.switchBackAdmin()

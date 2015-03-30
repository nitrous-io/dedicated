App.AdminDialogsView = Em.View.extend
  elementId: 'admin-dialogs-view'

  closeSwitchUserDidChange: (->
    if @get 'controller.closeSwitchUser'
      @set 'controller.closeSwitchUser', false
      @send 'cancelSwitchUser'
  ).observes 'controller.closeSwitchUser'

  didInsertElement: ->
    @_super()

    App.on 'show-update-hosts', (hosts) =>
      @set 'controller.hostsToUpdate', hosts
      if fVer = App.get('settings.freighterd_version')
        @set 'controller.freighterdVersion', fVer
      ui.showModal @$('#update-hosts')

    App.on 'hide-update-hosts', =>
      ui.hideModal @$('#update-hosts')
      @set 'controller.hostsToUpdate', null
      @set 'controller.freighterdVersion', null

    App.on 'open-switch-user-dialog', ->
      ui.showModal(@$('#switch-user-dialog'))
      @$('#switch-user-login').focus()

  willDestroyElement: ->
    @_super()
    App.off 'open-switch-user-dialog'

  actions:
    cancelUpdateHosts: ->
      App.trigger 'hide-update-hosts'

    cancelSwitchUser: ->
      ui.hideModal(@$('#switch-user-dialog'))

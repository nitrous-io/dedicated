App.AdminDialogsView = Em.View.extend
  elementId: 'admin-dialogs-view'

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

  actions:
    cancelUpdateHosts: ->
      App.trigger 'hide-update-hosts'

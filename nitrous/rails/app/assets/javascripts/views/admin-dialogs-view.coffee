App.AdminDialogsView = Em.View.extend
  elementId: 'admin-dialogs-view'

  closeSwitchUserDidChange: (->
    if @get 'controller.closeSwitchUser'
      @set 'controller.closeSwitchUser', false
      @send 'cancelSwitchUser'
  ).observes 'controller.closeSwitchUser'

  didInsertElement: ->
    @_super()

    App.on 'open-switch-user-dialog', ->
      ui.showModal(@$('#switch-user-dialog'))
      @$('#switch-user-login').focus()

    App.on 'open-view-user-dialog', =>
      ui.showModal(@$('#view-user-dialog'))
      @$('#view-user-login').focus()

    App.on 'hide-view-user-dialog', =>
      ui.hideModal @$('#view-user-dialog')
      @set 'controller.login', null

  willDestroyElement: ->
    @_super()
    App.off 'open-switch-user-dialog'

  actions:
    cancelSwitchUser: ->
      ui.hideModal(@$('#switch-user-dialog'))

    cancelViewUser: ->
      ui.hideModal(@$('#view-user-dialog'))

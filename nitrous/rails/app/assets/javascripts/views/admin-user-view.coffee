App.AdminUserView = Em.View.extend
  elementId: 'admin-user-view'
  classNames: ['admin', 'ui', 'card']

  banUserConfirmation: null
  banUser: null

  banUserConfirmationInvalid: (->
    @get('banUser.username') != @get('banUserConfirmation')
  ).property 'banUser.username', 'banUserConfirmation'

  clickBan: =>
    @$('.ban-btn').click()

  actions:
    switchUser: (user)->
      App.trigger 'switch-to-user', user.get('username')

    showBanDialog: (user)->
      @set 'banUser', user
      @set 'banUserConfirmation', null
      ui.showModal('#ban-user-modal-dialog')

    cancelBanDialog: ->
      @set 'banUser', null
      @set 'banUserConfirmation', null
      ui.hideModal('#ban-user-modal-dialog')

    confirmBan: ->
      ui.hideModal('#ban-user-modal-dialog')
      @get('controller').send 'banUser', @get('banUser')

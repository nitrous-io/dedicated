App.AccountView = Em.View.extend
  elementId: 'account-view'
  classNames: ['ui', 'card']

  deleteAccountConfirmation: null
  deleteAccountConfirmationInvalid: (->
    @get('deleteAccountConfirmation') != App.get("user.username")
  ).property 'deleteAccountConfirmation'

  emailFieldInvalid:    Em.computed.equal('controller.detailsInvalidField', 'email')
  passwordFieldInvalid: Em.computed.equal('controller.detailsInvalidField', 'password')

  didInsertElement: ->
    @_super()
    @$('[autofocus]').focus()

  showDeleteAccount: ->
    @set 'deleteAccountConfirmation', null
    ui.showModal('#deleteaccount-modal-dialog')

  cancelDeleteAccount: ->
    @set 'deleteAccountConfirmation', null
    ui.hideModal('#deleteaccount-modal-dialog')

  confirmDeleteAccount: ->
    ui.hideModal('#deleteaccount-modal-dialog')
    @get('controller').send 'deleteAccount'

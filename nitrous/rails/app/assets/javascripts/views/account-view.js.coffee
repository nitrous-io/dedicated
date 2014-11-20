App.AccountView = Em.View.extend
  elementId: 'account-view'
  classNames: ['ui', 'card']

  emailFieldInvalid:    Em.computed.equal('controller.detailsInvalidField', 'email')
  passwordFieldInvalid: Em.computed.equal('controller.detailsInvalidField', 'password')

  advancedDidChange: (->
    if @get 'controller.advanced'
      @send 'showModal'
  ).observes 'controller.advanced'

  showConfirmPassword: (->
    @get('controller.password') && @get('controller.password').length > 0
  ).property 'controller.password'

  actions:
    showModal: ->
      ui.showModal @$('#confirm-advanced-modal-dialog')

    hideModal: ->
      ui.hideModal @$('#confirm-advanced-modal-dialog')

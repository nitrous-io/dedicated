App.AccountView = Em.View.extend
  elementId: 'account-view'
  classNames: ['ui', 'card']

  advancedDidChange: (->
    if @get 'controller.advanced'
      @send 'showModal'
  ).observes 'controller.advanced'

  actions:
    showModal: ->
      ui.showModal @$('#confirm-advanced-modal-dialog')

    hideModal: ->
      ui.hideModal @$('#confirm-advanced-modal-dialog')

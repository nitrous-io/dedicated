App.BetaView = Em.View.extend
  elementId: 'beta-view'
  templateName: 'beta'

  closeDidChange: (->
    if @get 'controller.close'
      @set 'controller.close', false
      @send 'closeModal'
  ).observes 'controller.close'

  actions:
    sendFeedback: ->
      ui.showModal(@$('#feedback-dialog'))

    switchUser: ->
      ui.showModal(@$('#switch-user-dialog'))

    closeModal: ->
      ui.hideModal(@$('.ui.modal'))

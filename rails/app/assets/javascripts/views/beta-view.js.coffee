App.BetaView = Em.View.extend
  elementId: 'beta-view'
  templateName: 'beta'

  observeClose: (->
    if @get 'controller.close'
      @set 'controller.close', false
      @send 'cancelFeedback'
  ).observes 'controller.close'

  actions:
    sendFeedback: ->
      ui.showModal(@$('#feedback-dialog'))

    cancelFeedback: ->
      ui.hideModal(@$('#feedback-dialog'))

App.ApplyDialogMixin = Ember.Mixin.create
  closeApplyDialogDidChange: (->
    if @get 'controller.closeApplyDialog'
      @set 'controller.closeApplyDialog', false
      @send 'cancelApply'
  ).observes 'controller.closeApplyDialog'

  actions:
    askApply: ->
      ui.showModal(@$('#confirm-apply-modal-dialog'))

    cancelApply: ->
      ui.hideModal(@$('#confirm-apply-modal-dialog'))

    confirmApply: ->
      @get('controller').send 'apply'

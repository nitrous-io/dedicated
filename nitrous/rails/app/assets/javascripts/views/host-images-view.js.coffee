App.HostImagesView = Lib.RecordListView.extend
  elementId: 'host-images-view'

  pushButtonDisabled: (->
    !(@get('controller.image') && @get('controller.repo'))
  ).property 'controller.image', 'controller.repo'

  closePushDialogDidChange: (->
    if @get 'controller.closePushDialog'
      @set 'controller.closePushDialog', false
      @send 'cancelPush'
  ).observes 'controller.closePushDialog'

  actions:
    showPushDialog: (image) ->
      @set 'controller.image', image
      @set 'controller.repo', image.get('repo')
      ui.showModal('#push-modal-dialog')
      Em.run.scheduleOnce 'afterRender', =>
        @$('#push-repo').select()

    cancelPush: ->
      ui.hideModal('#push-modal-dialog')

    update: ->
      @$('input.record-checkbox').attr('checked', false).change()
      @get('controller').send('update')

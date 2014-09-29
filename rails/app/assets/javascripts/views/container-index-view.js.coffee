App.ContainerIndexView = Em.View.extend
  elementId: 'container-index-view'

  snapshotImageNameIsEmpty: Em.computed.empty 'controller.snapshotImageName'

  terminateSlugConfirmation: null

  terminateSlugConfirmationInvalid: (->
    @get('controller.model.slug') != @get('terminateSlugConfirmation')
  ).property 'controller.model.slug', 'terminateSlugConfirmation'

  closeSnapshotDialogDidChange: (->
    if @get 'controller.closeSnapshotDialog'
      @set 'controller.closeSnapshotDialog', false
      @send 'cancelSnapshot'
  ).observes 'controller.closeSnapshotDialog'


  actions:
    showSnapshotDialog: ->
      @set 'controller.snapshotImageName', makeDefaultImageName(@get 'controller.slug')
      ui.showModal('#snapshot-modal-dialog')
      Em.run.scheduleOnce 'afterRender', =>
        @$('#snapshot-name').select()

    cancelSnapshot: ->
      ui.hideModal('#snapshot-modal-dialog')

    confirmSnapshot: ->
      @get('controller').send 'snapshot'

    showTerminateDialog: ->
      @set 'terminateSlugConfirmation', null
      ui.showModal('#terminate-modal-dialog')

    cancelTerminate: ->
      ui.hideModal('#terminate-modal-dialog')

    confirmTerminate: ->
      ui.hideModal('#terminate-modal-dialog')
      @get('controller').send 'terminate'

  clickTerminate: =>
    @$('.terminate-btn').click()

lpadZero = (n) ->
  return n if n > 9
  "0#{n}"

makeDefaultImageName = (slug) ->
  d = new Date
  "#{slug}:snapshot-#{d.getFullYear()}#{lpadZero(d.getMonth() + 1)}#{lpadZero(d.getDate())}-#{lpadZero(d.getHours())}#{lpadZero(d.getMinutes())}#{lpadZero(d.getSeconds())}"

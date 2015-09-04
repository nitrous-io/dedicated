App.ContainerIndexView = Em.View.extend
  elementId: 'container-index-view'

  terminateSlugConfirmation: null

  terminateSlugConfirmationInvalid: (->
    @get('controller.model.slug') != @get('terminateSlugConfirmation')
  ).property 'controller.model.slug', 'terminateSlugConfirmation'

  snapshotImageNameEmpty: Em.computed.empty 'controller.snapshotImageName'

  closeSnapshotDialogDidChange: (->
    if @get 'controller.closeSnapshotDialog'
      @set 'controller.closeSnapshotDialog', false
      @send 'cancelSnapshot'
  ).observes 'controller.closeSnapshotDialog'

  didInsertElement: ->
    @_super()
    if App.get('sharedContainerTip')
      App.set 'sharedContainerTip', false
      App.set 'onboardingScreen', 'shared-container'

  actions:
    showTerminateDialog: ->
      @set 'terminateSlugConfirmation', null
      ui.showModal('#terminate-modal-dialog')

    cancelTerminate: ->
      ui.hideModal('#terminate-modal-dialog')

    confirmTerminate: ->
      ui.hideModal('#terminate-modal-dialog')
      @get('controller').send 'terminate'

    showSnapshotDialog: ->
      @set 'controller.snapshotImageName', @get 'controller.slug'
      ui.showModal('#snapshot-modal-dialog')
      Em.run.scheduleOnce 'afterRender', =>
        @$('#snapshot-name').select()

    cancelSnapshot: ->
      ui.hideModal('#snapshot-modal-dialog')

    confirmSnapshot: ->
      @get('controller').send 'snapshot'

  clickTerminate: =>
    @$('.terminate-btn').click()

lpadZero = (n) ->
  return n if n > 9
  "0#{n}"

makeDefaultImageName = (slug) ->
  d = new Date
  "#{slug}:snapshot-#{d.getFullYear()}#{lpadZero(d.getMonth() + 1)}#{lpadZero(d.getDate())}-#{lpadZero(d.getHours())}#{lpadZero(d.getMinutes())}#{lpadZero(d.getSeconds())}"

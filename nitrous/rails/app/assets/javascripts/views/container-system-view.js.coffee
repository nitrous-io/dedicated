App.ContainerSystemView = Em.View.extend
  elementId: 'container-system-view'

  terminateSlugConfirmation: null

  terminateSlugConfirmationInvalid: (->
    @get('controller.model.slug') != @get('terminateSlugConfirmation')
  ).property 'controller.model.slug', 'terminateSlugConfirmation'

  actions:
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

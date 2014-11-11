App.UserSettingsView = Em.View.extend
  elementId: 'user-settings-view'
  classNames: ['ui', 'card']

  didInsertElement: ->
    @_super()
    @$().on 'change', 'input#user-settings-advanced', (event) =>
      if $(event.target).is(':checked')
        @send 'showModal'

  actions:
    showModal: ->
      ui.showModal @$('#confirm-advanced-modal-dialog')

    hideModal: ->
      ui.hideModal @$('#confirm-advanced-modal-dialog')

App.ContainerKeysNewView = Em.View.extend
  elementId: 'container-keys-new-view'
  classNames: ['keys-new-view']

  createButtonDisabled: Em.computed.empty 'controller.key'

  didInsertElement: ->
    @_super()
    @$('textarea').autosize()
    @$('#keys-new-key').focus()

  willDestroyElement: ->
    @_super()
    @$('textarea').trigger('autosize.destroy')

App.ContainerKeysNewView = Em.View.extend
  elementId: 'container-keys-new-view'

  didInsertElement: ->
    @_super()
    @$('textarea').autosize()

  willDestroyElement: ->
    @_super()
    @$('textarea').trigger('autosize.destroy')
